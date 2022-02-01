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

type VmStartCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewVmStartCommand(ui cli.Ui, trellis *trellis.Trellis) *VmStartCommand {
	c := &VmStartCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmStartCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *VmStartCommand) Run(args []string) int {
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

	if err := lima.Installed(); err != nil {
		c.UI.Error(err.Error())
		c.UI.Error("Install or upgrade Lima to continue:")
		c.UI.Error("\n  brew install lima\n")
		c.UI.Error("See https://github.com/lima-vm/lima#getting-started for manual installation options")
		return 1
	}

	instance := manager.NewInstance(siteName)
	existingInstance, ok := manager.GetInstance(siteName)

	if ok {
		instance = existingInstance

		if instance.Running() {
			c.UI.Info(fmt.Sprintf("%s VM already running", color.GreenString("[✓]")))
		} else {
			if err := instance.Start(c.UI); err != nil {
				c.UI.Error("Error starting virtual machine.")
				c.UI.Error(err.Error())
				return 1
			}
		}

		if err := instance.Hydrate(true); err != nil {
			c.UI.Error("Error getting VM info. This is a trellis-cli bug.")
			c.UI.Error(err.Error())
			return 1
		}

		if err = c.writeFiles(manager, instance); err != nil {
			c.UI.Error(err.Error())
			return 1
		}

		return 0
	}

	c.UI.Info("Creating new VM...")
	if err := instance.Create(); err != nil {
		c.UI.Error("Error creating VM.")
		c.UI.Error(err.Error())
		return 1
	}

	if err := instance.Hydrate(true); err != nil {
		c.UI.Error("Error getting VM info. This is a trellis-cli bug.")
		c.UI.Error(err.Error())
		return 1
	}

	if err = c.writeFiles(manager, instance); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info("\nProvisioning VM...")

	provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
	return provisionCmd.Run([]string{"development"})
}

func (c *VmStartCommand) Synopsis() string {
	return "Starts a development virtual machine."
}

func (c *VmStartCommand) Help() string {
	helpText := `
Usage: trellis vm start [options]

Starts a development virtual machine.
If a VM doesn't exist yet, it will be created. If a VM already exists, it will be started.

Note: VM management (under the 'trellis vm' subcommands) is currently only available for macOS Ventura (13.0) and later.
Lima (https://lima-vm.io/) is the underlying VM manager which requires macOS's new virtualization framework.

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VmStartCommand) writeFiles(manager *lima.Manager, instance lima.Instance) error {
	if err := instance.CreateInventoryFile(); err != nil {
		return err
	}

	if err := manager.HostsResolver.AddHosts(instance.Name, &instance); err != nil {
		return err
	}

	return nil
}
