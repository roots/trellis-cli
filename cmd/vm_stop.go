package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/lima"
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

	siteName, err := c.Trellis.FindSiteNameFromEnvironment("development", "")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	manager, err := lima.NewManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	instance, ok := manager.GetInstance(siteName)

	if !ok {
		c.UI.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
	} else {
		if instance.Stopped() {
			c.UI.Info(fmt.Sprintf("%s VM already stopped", color.GreenString("[✓]")))
		} else {
			if err := manager.StopInstance(instance); err != nil {
				c.UI.Error("Error stopping VM")
				c.UI.Error(err.Error())
				return 1
			}
		}
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

	return strings.TrimSpace(helpText)
}
