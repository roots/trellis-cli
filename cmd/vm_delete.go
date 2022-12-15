package cmd

import (
	"flag"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/lima"
	"github.com/roots/trellis-cli/trellis"
)

type VmDeleteCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewVmDeleteCommand(ui cli.Ui, trellis *trellis.Trellis) *VmDeleteCommand {
	c := &VmDeleteCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmDeleteCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *VmDeleteCommand) Run(args []string) int {
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

	manager, err := lima.NewManager(c.Trellis)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	instance, ok := manager.GetInstance(siteName)

	if !ok {
		c.UI.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return 0
	}

	if err := instance.Hydrate(false); err != nil {
		c.UI.Error("Error getting VM info. This is a trellis-cli bug.")
		c.UI.Error(err.Error())
		return 1
	}

	if instance.Stopped() {
		if err := instance.Delete(); err != nil {
			c.UI.Error(err.Error())
			return 1
		}
	} else {
		c.UI.Error("Error: VM is running. Run `trellis vm stop` to stop it.")
		return 1
	}

	return 0
}

func (c *VmDeleteCommand) Synopsis() string {
	return "Deletes the development virtual machine."
}

func (c *VmDeleteCommand) Help() string {
	helpText := `
Usage: trellis vm delete [options]

Deletes the development virtual machine.

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}
