package cmd

import (
	"errors"
	"flag"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

type VmStartCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	name    string
}

func NewVmStartCommand(ui cli.Ui, trellis *trellis.Trellis) *VmStartCommand {
	c := &VmStartCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmStartCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.name, "name", "", "Name of the VM")
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

	if c.name == "" {
		siteName, err := c.Trellis.FindSiteNameFromEnvironment("development", "")

		if err == nil {
			c.name = siteName
		} else {
			c.UI.Warn("Warning: default VM name could not be automatically determined.")

			prompt := promptui.Prompt{
				Label: "VM name",
			}

			nameInput, err := prompt.Run()
			if err != nil {
				c.UI.Error("Aborting: no name provided")
				return 1
			}

			c.name = nameInput
		}
	}

	manager, err := newVmManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	err = manager.StartInstance(c.name)
	if err == nil {
		c.printInstanceInfo()
		return 0
	}

	if !errors.Is(err, vm.VmNotFoundErr) {
		c.UI.Error("Error starting VM.")
		c.UI.Error(err.Error())
		return 1
	}

	// VM doesn't exist yet, create it
	if err = manager.CreateInstance(c.name); err != nil {
		c.UI.Error("Error creating VM.")
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info("\nProvisioning VM...")

	provisionCmd := NewProvisionCommand(c.UI, c.Trellis)
	code := provisionCmd.Run([]string{"development"})

	if code == 0 {
		c.printInstanceInfo()
	}

	return code
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

Start a VM with a custom name:

  $ trellis vm start --name my-vm

Options:
      --name  Name of the VM (default: main development site name)
  -h, --help  Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VmStartCommand) printInstanceInfo() {
	c.UI.Info(`
Your Trellis VM is ready to use!

* Composer and WP-CLI commands need to be run on the virtual machine for any post-provision modifications.
* You can SSH into the machine with 'trellis vm shell'
* Then navigate to your WordPress sites at '/srv/www'`)
}
