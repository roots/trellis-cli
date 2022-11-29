package cmd

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/go-homedir"
	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/hostagent"
	"github.com/roots/trellis-cli/http-proxy"
	"github.com/roots/trellis-cli/lima"
	"github.com/roots/trellis-cli/trellis"
)

type VmStartCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewVmStartCommand(ui cli.Ui, trellis *trellis.Trellis) *VmStartCommand {
	c := &VmStartCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmStartCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *VmStartCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	dataDir := app_paths.DataDir()
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		c.UI.Error("Error creating trellis-cli data dir.")
		c.UI.Error(err.Error())
		return 1
	}

	osPath := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:%s", dataDir, osPath))
	if err := c.installLima(dataDir); err != nil {
		return 1
	}

	if hostagent.Running() {
		c.UI.Info(fmt.Sprintf("%s hostagent running", color.GreenString("[✓]")))
	} else {
		if err := c.hostagentInstall(); err != nil {
			return 1
		}
	}

	limaConfigPath := filepath.Join(c.Trellis.ConfigPath(), "lima")
	os.MkdirAll(limaConfigPath, 0755)

	siteName, err := c.Trellis.FindSiteNameFromEnvironment("development", "")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	sites := c.Trellis.Environments["development"].WordPressSites

	var firstRun bool = false
	var instance lima.Instance
	instance, ok := lima.GetInstance(siteName)

	if ok {
		if err := instance.Start(); err != nil {
			c.UI.Error("Error starting virtual machine.")
			c.UI.Error(err.Error())
			return 1
		}
	} else {
		c.UI.Info("Creating new Lima VM...")
		firstRun = true
		instance = lima.NewInstance(siteName, limaConfigPath, sites)

		if err := instance.Create(); err != nil {
			c.UI.Error("Error creating VM.")
			c.UI.Error(err.Error())
			return 1
		}
	}

	if err := instance.Hydrate(); err != nil {
		c.UI.Error("Error getting VM info. This is a trellis-cli bug.")
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("\n%s Lima VM started\n", color.GreenString("[✓]")))
	c.UI.Info(fmt.Sprintf("Name: %s", instance.Name))
	c.UI.Info(fmt.Sprintf("Local SSH port: %d", instance.SshLocalPort))
	c.UI.Info(fmt.Sprintf("Local HTTP port: %d", instance.HttpForwardPort))

	hostNames := c.Trellis.Environments["development"].AllHosts()
	proxyHost := fmt.Sprintf("http://127.0.0.1:%d", instance.HttpForwardPort)

	if err := httpProxy.AddRecords(proxyHost, hostNames); err != nil {
		c.UI.Error("Error writing hosts files for HTTP proxy. This is probably a trellis-cli bug; please report it.")
		return 1
	}

	sshConfigPath := filepath.Join(limaConfigPath, "ssh_config")
	if err = addSshConfigInclude(sshConfigPath); err != nil {
		c.UI.Error("Error adding include directive to ~/.ssh/config")
		c.UI.Error(err.Error())
		return 1
	}

	err = createSshConfig(sshConfigPath, instance.Name)
	if err != nil {
		c.UI.Error("Error creating SSH config")
		c.UI.Error(err.Error())
		return 1
	}

	hostsPath := filepath.Join(limaConfigPath, "inventory")
	if err = createInventoryFile(hostsPath, instance); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if firstRun {
		c.UI.Info("\nProvisioning VM...")

		os.Setenv("ANSIBLE_HOST_KEY_CHECKING", "false")
		provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
		return provisionCmd.Run([]string{"--extra-vars", "web_user=" + instance.Username, "development"})
	} else {
		c.UI.Info("\nSkipping provisioning. VM already created.")
		c.UI.Info("To provision again, run: trellis provision development")
	}

	return 0
}

func (c *VmStartCommand) Synopsis() string {
	return "Starts a Trellis development virtual machine."
}

func (c *VmStartCommand) Help() string {
	helpText := `
Usage: trellis vm start [options]

Starts a Trellis development virtual machine.

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VmStartCommand) installLima(path string) error {
	if _, err := exec.LookPath("limactl"); err != nil {
		spinner := NewSpinner(
			SpinnerCfg{
				Message:     "Installing lima",
				FailMessage: "Error installing lima",
			},
		)
		spinner.Start()
		err := lima.Install(path)

		if err != nil {
			spinner.StopFail()
			c.UI.Error(err.Error())
			return err
		}

		spinner.Stop()
	}

	return nil
}

func (c *VmStartCommand) hostagentInstall() error {
	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Checking hostagent requirements",
			FailMessage: "hostagent requiremnts not met",
		},
	)

	spinner.Start()
	portsInUse := hostagent.PortsInUse()

	if len(portsInUse) > 0 {
		spinner.StopFail()
		c.UI.Error("The hostagent runs a reverse HTTP proxy and a local DNS resolver and requires a few ports to be free. The following ports are already in use by another service on your machine:")

		for _, port := range portsInUse {
			c.UI.Error(fmt.Sprintf("%s %d", port.Protocol, port.Number))
		}

		c.UI.Error("\nUsing the `lsof` command will let you know what process is using the port:")
		for _, port := range portsInUse {
			c.UI.Error(fmt.Sprintf("=> sudo lsof -nP -iTCP:%d -sTCP:LISTEN", port.Number))
		}

		return fmt.Errorf("install failed")
	}

	spinner.Stop()

	if hostagent.Installed() {
		if err := c.runHostagent(); err != nil {
			return err
		}
	} else {
		c.UI.Info(fmt.Sprintf("%s hostagent not installed", color.RedString("[✘]")))
		c.UI.Info("\ntrellis-cli hostagent needs to be installed on your host machine for VM integration. The hostagent is a service that runs in the background with a reverse HTTP proxy and a local DNS resolver.")
		c.UI.Info("The DNS resolver will resolve queries for the *.test domain and always respond with 127.0.0.1. Using a DNS resolver means your /etc/hosts does not need to be updated.")
		c.UI.Info("The HTTP server runs on port 80 and proxies requests from site hosts to the VM's forward port. Example: example.test:80 -> 127.0.0.1:63208")
		c.UI.Info("\nTwo files are created as part of the installation:")
		c.UI.Info(" 1. " + hostagent.PlistPath())
		c.UI.Info(" 2. " + hostagent.ResolverPath())
		c.UI.Info("\nNote: sudo is needed to create the resolver file. The hostagent service will run under your user account, not as root.")

		if err := hostagent.Install(); err != nil {
			c.UI.Error("Error installing trellis-cli hostagent service.")
			c.UI.Error(err.Error())
			return err
		}
	}

	return nil
}

func (c *VmStartCommand) runHostagent() error {
	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Starting hostagent",
			FailMessage: "Hostagent could not start",
		},
	)

	spinner.Start()
	if err := hostagent.RunServer(); err != nil {
		spinner.StopFail()
		return err
	}

	spinner.Stop()
	return nil
}

func createInventoryFile(path string, instance lima.Instance) error {
	const hostsTemplate string = `
[development]
127.0.0.1 ansible_port={{ .SshLocalPort }} ansible_user={{ .Username }}

[web]
127.0.0.1 ansible_port={{ .SshLocalPort }} ansible_user={{ .Username }}
`

	tpl := template.Must(template.New("hosts").Parse(hostsTemplate))

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("Could not create Ansible inventory file: %v", err)
	}

	err = tpl.Execute(file, instance)
	if err != nil {
		return fmt.Errorf("Could not create Ansible inventory file: %v", err)
	}

	return nil
}

func addSshConfigInclude(includesPath string) error {
	includeStatement := fmt.Sprintf("Include %s\n\n", includesPath)
	homePath, _ := homedir.Dir()
	path := filepath.Join(homePath, ".ssh", "config")

	configContents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("Could not read ~/.ssh/config: %v", err)
	}

	if !strings.Contains(string(configContents), includeStatement) {
		err = os.WriteFile(path, []byte(includeStatement+string(configContents)), 0644)
		if err != nil {
			return fmt.Errorf("Could not write ~/.ssh/config: %v", err)
		}
	}

	return nil
}

func createSshConfig(path string, instanceName string) error {
	sshConfig, err := command.Cmd("limactl", []string{"show-ssh", "--format=config", instanceName}).Output()
	if err != nil {
		return fmt.Errorf("Could not fetch lima SSH config: %v", err)
	}

	re := regexp.MustCompile(`User (.*)`)
	sshConfigWeb := re.ReplaceAll([]byte(sshConfig), []byte("User web"))

	re = regexp.MustCompile(`Host (.*)`)
	sshConfigWeb = re.ReplaceAll([]byte(sshConfigWeb), []byte("Host $1-web"))

	re = regexp.MustCompile(`[\s]+(ControlPath .*)`)
	sshConfigWeb = re.ReplaceAll([]byte(sshConfigWeb), []byte(""))

	contents := string(sshConfig) + "\n\n" + string(sshConfigWeb)
	err = os.WriteFile(path, []byte(contents), 0644)
	if err != nil {
		return fmt.Errorf("Could not write SSH config to %s: %v", path, err)
	}

	return nil
}
