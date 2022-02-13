package cmd

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/template"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/go-homedir"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/config"
	"github.com/roots/trellis-cli/github"
	"github.com/roots/trellis-cli/http-proxy"
	"github.com/roots/trellis-cli/lima"
	"github.com/roots/trellis-cli/trellis"
)

type StartCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewStartCommand(ui cli.Ui, trellis *trellis.Trellis) *StartCommand {
	c := &StartCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *StartCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *StartCommand) Run(args []string) int {
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

	dataDirs, err := config.Scope.DataDirs()
	if err != nil {
		c.UI.Error("could not determine XDG data dir. This is a trellis-cli bug.")
		return 1
	}

	dataDir := dataDirs[0]
	if err := os.MkdirAll(dataDir, os.ModePerm); err != nil {
		c.UI.Error("Error creating trellis-cli data dir.")
		c.UI.Error(err.Error())
		return 1
	}

	osPath := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:%s", dataDir, osPath))

	if _, err := exec.LookPath("limactl"); err != nil {
		spinner := NewSpinner(
			SpinnerCfg{
				Message:     "Installing lima",
				FailMessage: "Error installing lima",
			},
		)
		spinner.Start()
		err := lima.Install(dataDir)

		if err != nil {
			spinner.StopFail()
			c.UI.Error(err.Error())

			return 1
		}

		spinner.Stop()
	}

	if _, err := exec.LookPath("mutagen"); err != nil {
		spinner := NewSpinner(
			SpinnerCfg{
				Message:     "Installing mutagen",
				FailMessage: "Error installing mutagen",
			},
		)
		spinner.Start()
		installMutagen(dataDir)

		if err != nil {
			spinner.StopFail()
			c.UI.Error(err.Error())

			return 1
		}

		spinner.Stop()
	}

	siteName, err := c.Trellis.FindSiteNameFromEnvironment("development", "")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := httpProxy.Install(); err != nil {
		c.UI.Error("Error installing HTTP proxy launch agent.")
		c.UI.Error(err.Error())
		return 1
	}

	limaConfigPath := filepath.Join(c.Trellis.ConfigPath(), "lima")
	os.MkdirAll(limaConfigPath, os.ModePerm)

	var firstRun bool = false
	var instance *lima.Instance

	if lima.InstanceExists(siteName) {
		instance = lima.NewInstance(siteName, limaConfigPath)
		if err := instance.Start(); err != nil {
			c.UI.Error("Error starting VM.")
			c.UI.Error(err.Error())
			return 1
		}
	} else {
		c.UI.Info("Creating new Lima VM...")
		instance = lima.NewInstance(siteName, limaConfigPath)
		firstRun = true
		if err := instance.Create(); err != nil {
			c.UI.Error("Error creating VM.")
			c.UI.Error(err.Error())
			return 1
		}
	}

	instance, err = lima.HydrateInstance(siteName, limaConfigPath)
	if err != nil {
		c.UI.Error("Error getting VM info. This is a trellis-cli bug.")
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("\n%s Lima VM started\n", color.GreenString("[✓]")))
	c.UI.Info(fmt.Sprintf("Name: %s", instance.Name))
	c.UI.Info(fmt.Sprintf("Local SSH port: %d", instance.SshLocalPort))
	c.UI.Info(fmt.Sprintf("Local HTTP port: %d", instance.HttpForwardPort))

	err = writeProxyRecords(dataDir, instance, c.Trellis.Environments["development"].AllHosts())
	if err != nil {
		c.UI.Error("Error writing hosts files for HTTP proxy. This is a trellis-cli bug; please report it.")
		return 1
	}

	sshConfigPath := filepath.Join(limaConfigPath, "ssh_config")
	if err = addSshConfigInclude(sshConfigPath); err != nil {
		c.UI.Error("Error adding SSH config include")
		c.UI.Error(err.Error())
		return 1
	}

	err = createSshConfig(sshConfigPath, instance.Name)
	if err != nil {
		c.UI.Error("Error creating SSH configs")
		c.UI.Error(err.Error())
		return 1
	}

	err = command.Cmd("mutagen", []string{"sync", "list", instance.Name}).Run()

	if err != nil {
		site, _ := c.Trellis.Environments["development"].WordPressSites[siteName]
		sitePath := fmt.Sprintf("/srv/www/%s/current", siteName)

		mutagenArgs := []string{
			"sync",
			"create",
			site.LocalPath,
			fmt.Sprintf("lima-%s-%s:%s", instance.Name, "web", sitePath),
			"--name=" + instance.Name,
			"--default-owner-beta=web",
			"--default-group-beta=www-data",
			"--default-file-mode-beta=0644",
			"--default-directory-mode-beta=0755",
		}

		err = command.WithOptions(command.WithTermOutput(), command.WithLogging(c.UI)).Cmd("mutagen", mutagenArgs).Run()
	}

	hostsPath := filepath.Join(limaConfigPath, "inventory")
	err = createLimaHostsFile(hostsPath, instance.SshLocalPort)

	if firstRun {
		c.UI.Info("\nProvisioning VM...")

		os.Setenv("ANSIBLE_HOST_KEY_CHECKING", "false")
		provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
		return provisionCmd.Run([]string{"--extra-vars", "web_user=web", "development"})
	} else {
		c.UI.Info("\nSkipping provisioning. VM already created.")
		c.UI.Info("To provision again, run: trellis provision development")
	}

	return 0
}

func (c *StartCommand) Synopsis() string {
	return "Starts a VM and provisions the server with Trellis"
}

func (c *StartCommand) Help() string {
	helpText := `
Usage: trellis start [options]

Starts a VM and provisions the server with Trellis

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}

func installMutagen(installPath string) error {
	tempDir, _ := ioutil.TempDir("", "trellis-mutagen")
	defer os.RemoveAll(tempDir)

	pattern := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)

	github.DownloadAsset(
		"mutagen-io/mutagen",
		"latest",
		tempDir,
		tempDir,
		pattern,
	)

	files, err := os.ReadDir(tempDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		err := os.Rename(filepath.Join(tempDir, file.Name()), filepath.Join(installPath, file.Name()))
		if err != nil {
			return err
		}
	}

	return nil
}

func createLimaHostsFile(path string, port int) error {
	const hostsTemplate string = `
[development]
127.0.0.1 ansible_port={{ .Port }}

[web]
127.0.0.1 ansible_port={{ .Port }}
`

	tpl := template.Must(template.New("hosts").Parse(hostsTemplate))

	file, err := os.Create(path)
	if err != nil {
		return err
	}

	data := struct {
		Port int
	}{
		Port: port,
	}

	err = tpl.Execute(file, data)
	if err != nil {
		return err
	}

	return nil
}

func addSshConfigInclude(includesPath string) error {
	includeStatement := fmt.Sprintf("Include %s\n\n", includesPath)
	homePath, _ := homedir.Dir()
	path := filepath.Join(homePath, ".ssh", "config")

	configContents, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	if !strings.Contains(string(configContents), includeStatement) {
		os.WriteFile(path, []byte(includeStatement+string(configContents)), 0644)
	}

	return nil
}

func createSshConfig(path string, instanceName string) error {
	sshConfig, err := command.Cmd("limactl", []string{"show-ssh", "--format=config", instanceName}).Output()
	if err != nil {
		return err
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
		return err
	}

	return nil
}

func writeProxyRecords(dataDir string, instance *lima.Instance, hosts []string) (err error) {
	for _, host := range hosts {
		path := filepath.Join(dataDir, host)
		contents := []byte(fmt.Sprintf("http://127.0.0.1:%d", instance.HttpForwardPort))
		err = os.WriteFile(path, contents, 0644)

		if err != nil {
			return err
		}
	}

	return nil
}
