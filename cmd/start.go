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

	"github.com/mitchellh/cli"
	"github.com/mitchellh/go-homedir"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/github"
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

	if _, err := exec.LookPath("limactl"); err != nil {
		spinner := NewSpinner(
			SpinnerCfg{
				Message:     "Installing lima",
				FailMessage: "Error installing lima",
			},
		)
		spinner.Start()
		err := lima.Install(c.Trellis.Virtualenv.BinPath)

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
		installMutagen(c)

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

	limaInstanceName := lima.ConvertToInstanceName(siteName)
	limaConfigPath := filepath.Join(c.Trellis.ConfigPath(), "lima")
	os.MkdirAll(limaConfigPath, os.ModePerm)

	configFilePath := filepath.Join(limaConfigPath, limaInstanceName+".yml")

	if err = lima.CreateConfig(configFilePath, siteName); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	instance, ok := lima.GetInstance(limaInstanceName)
	var configOrName string

	if ok {
		configOrName = instance.Name
	} else {
		configOrName = configFilePath
	}

	err = command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(c.UI),
	).Cmd("limactl", []string{"start", "--tty=false", configOrName}).Run()

	if err != nil {
		c.UI.Error("Error starting lima instance.")
		c.UI.Error(err.Error())
		return 1
	}

	instance, _ = lima.GetInstance(limaInstanceName)

	c.UI.Info(fmt.Sprintf("\nLima VM instance started: %s\n", instance.Name))

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

	c.UI.Info("\nProvisioning VM...")

	os.Setenv("ANSIBLE_HOST_KEY_CHECKING", "false")
	provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
	return provisionCmd.Run([]string{"--extra-vars", "web_user=web", "development"})
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

func installMutagen(c *StartCommand) error {
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
		err := os.Rename(filepath.Join(tempDir, file.Name()), filepath.Join(c.Trellis.Virtualenv.BinPath, file.Name()))
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
	err = os.WriteFile(path, []byte(contents), os.ModePerm)
	if err != nil {
		return err
	}

	return nil
}
