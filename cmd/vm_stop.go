package cmd

import (
	"flag"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

type VmStopCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewVmStopCommand(ui cli.Ui, trellis *trellis.Trellis) *VmStopCommand {
	c := &VmStopCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmStopCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *VmStopCommand) Run(args []string) int {
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

	instanceName, err := c.Trellis.GetVmInstanceName()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	manager, err := newVmManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	if err := manager.StopInstance(instanceName); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *VmStopCommand) Synopsis() string {
	return "Stops the development virtual machine."
}

func (c *VmStopCommand) Help() string {
	helpText := `
Usage: trellis vm stop [options]

Stops the development virtual machine.

Options:
  -h, --help show this help
`

	return CreateHelp("vm stop", c.Synopsis(), strings.TrimSpace(helpText))
}
