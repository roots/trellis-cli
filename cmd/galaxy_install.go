package cmd

import (
	"fmt"
	"os"
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

	galaxyInstall := execCommand("ansible-galaxy", "install", "-r", files[0])
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
