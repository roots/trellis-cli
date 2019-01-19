package cmd

import (
	"github.com/mitchellh/cli"
	"log"
	"os/exec"
	"strings"
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

	galaxyInstall := exec.Command("ansible-galaxy", "install", "-r", "requirements.yml")
	logCmd(galaxyInstall, true)
	err := galaxyInstall.Run()

	if err != nil {
		log.Fatal(err)
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
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}
