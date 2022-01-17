package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type GalaxyInstallCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

var RoleAlreadyInstalledPattern = regexp.MustCompile(`^.*\[WARNING\]\: - (.*) \(.*\) .*`)

func (c *GalaxyInstallCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	var roleFiles = []string{"galaxy.yml", "requirements.yml"}
	var files []string

	for _, file := range roleFiles {
		if _, err := os.Stat(file); !os.IsNotExist(err) {
			files = append(files, file)
		}
	}

	switch len(files) {
	case 0:
		c.UI.Error("Error: no role file found")
		return 1
	case 2:
		c.UI.Warn("Warning: multiple role files found. Defaulting to galaxy.yml")
	}

	mockUi := cli.NewMockUi()

	galaxyInstall := command.WithOptions(
		command.WithUiOutput(mockUi),
	).Cmd("ansible-galaxy", []string{"install", "-r", files[0]})

	err := galaxyInstall.Run()

	if err != nil {
		c.UI.Error(mockUi.ErrorWriter.String())
		c.UI.Error(fmt.Sprintf("Error running ansible-galaxy: %s", err))
		return 1
	}

	c.UI.Info(mockUi.OutputWriter.String())

	var rolesToForceUpdate []string

	s := bufio.NewScanner(bytes.NewReader(mockUi.ErrorWriter.Bytes()))
	s.Split(bufio.ScanLines)

	for s.Scan() {
		match := RoleAlreadyInstalledPattern.FindStringSubmatch(s.Text())

		if len(match) > 0 {
			rolesToForceUpdate = append(rolesToForceUpdate, match[1])
		}
	}

	if len(rolesToForceUpdate) > 0 {
		c.UI.Info(fmt.Sprintf("Updating roles: %s\n", strings.Join(rolesToForceUpdate, ", ")))
		installArgs := append([]string{"install", "-f", "-r", files[0]}, rolesToForceUpdate...)
		galaxyInstall := command.WithOptions(command.WithLogging(c.UI)).Cmd("ansible-galaxy", installArgs)
		err = galaxyInstall.Run()

		if err != nil {
			c.UI.Error(fmt.Sprintf("Error running ansible-galaxy: %s", err))
			return 1
		}
	}

	return 0
}

func (c *GalaxyInstallCommand) Synopsis() string {
	return "Installs Ansible Galaxy roles"
}

func (c *GalaxyInstallCommand) Help() string {
	helpText := `
Usage: trellis galaxy install

Installs Ansible Galaxy roles.

See https://docs.roots.io/trellis/master/remote-server-setup/#requirements for more information.

Any previously installed roles that have a new version updated in galaxy.yml will automatically be installed.

This means there's no need to use the --force option with ansible-galaxy to get a newer version of a role.
If you want to clear out all the roles and re-install them all it's recommended to simply delete your vendor directory.

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
