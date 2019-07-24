package cmd

import (
	"fmt"
	"strings"

	"trellis-cli/trellis"

	"github.com/mitchellh/cli"
)

type DownCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *DownCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	switch len(args) {
	case 0:
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 0, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	vagrantArgs := []string{"halt"}

	vagrantHalt := execCommand("vagrant", vagrantArgs...)

	logCmd(vagrantHalt, c.UI, true)
	err := vagrantHalt.Run()
	if err != nil {
		return 1
	}

	return 0
}

func (c *DownCommand) Synopsis() string {
	return "Stops the Vagrant machine by running 'vagrant halt'."
}

func (c *DownCommand) Help() string {
	helpText := `
Usage: trellis down [options]

Stops the Vagrant machine by running 'vagrant halt'.

Stop Vagrant VM:

  $ trellis down

Options:
  -h, --help         show this help
`

	return strings.TrimSpace(helpText)
}
