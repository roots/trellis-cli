package cmd

import (
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

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	vagrantHalt := execCommandWithOutput("vagrant", []string{"halt"}, c.UI)
	err := vagrantHalt.Run()

	if err != nil {
		return 1
	}

	return 0
}

func (c *DownCommand) Synopsis() string {
	return "Stops the Vagrant machine by running 'vagrant halt'"
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
