package cmd

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type GalaxyInstallCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *GalaxyInstallCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if len(args) > 0 {
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 0, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	galaxyInstall := execCommand("ansible-galaxy", "install", "-r", "requirements.yml")
	logCmd(galaxyInstall, c.UI, true)
	err := galaxyInstall.Run()

	if err != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-galaxy: %s", err))
		return 1
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

See https://roots.io/trellis/docs/remote-server-setup/#requirements for more information.

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
