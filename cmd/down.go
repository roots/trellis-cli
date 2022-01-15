package cmd

import (
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
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

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	vagrantHalt := command.WithOptions(
		command.WithLogging(c.UI),
		command.WithTermOutput(),
	).Cmd("vagrant", []string{"halt"})

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
