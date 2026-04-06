package cmd

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/pkg/wsl"
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

	if windowsHostRequired(c.Trellis, c.UI, "vm start") {
		return 1
	}

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

	err = manager.StartInstance(instanceName)
	if err == nil {
		// If the distro exists but was never fully provisioned (e.g. user
		// cancelled during bootstrap), clean it up and re-create.
		if wslManager, ok := manager.(*wsl.Manager); ok && !wslManager.IsProvisioned(instanceName) {
			c.UI.Warn("Detected unprovisioned WSL distro. Cleaning up and starting fresh...")
			_ = manager.DeleteInstance(instanceName)
		} else {
			c.printInstanceInfo()
			return 0
		}
	}

	if err != nil && !errors.Is(err, vm.ErrVmNotFound) {
		c.UI.Error("Error starting VM.")
		c.UI.Error(err.Error())
		return 1
	}

	// VM doesn't exist yet, create and start it
	if err = manager.CreateInstance(instanceName); err != nil {
		c.UI.Error("Error creating VM.")
		c.UI.Error(err.Error())
		return 1
	}

	if err = manager.StartInstance(instanceName); err != nil {
		c.UI.Error("Error starting VM.")
		c.UI.Error(err.Error())
		return 1
	}

	fmt.Print("\r\n")
	c.UI.Info("Provisioning VM...")

	// For WSL, provisioning runs inside the distro (no host-side Ansible).
	// We bootstrap Ansible into the distro first, then run the playbook.
	if wslManager, ok := manager.(*wsl.Manager); ok {
		if err := wslManager.BootstrapInstance(instanceName); err != nil {
			c.UI.Error("Error bootstrapping VM.")
			c.UI.Error(err.Error())
			return 1
		}

		if err := wslManager.Provision(instanceName); err != nil {
			c.UI.Error("Error provisioning VM.")
			c.UI.Error(err.Error())
			return 1
		}

		c.printInstanceInfo()
		return 0
	}

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

Lima (https://lima-vm.io/) is the underlying VM manager.
Local VM support requires macOS 13.0+ or Linux with Lima and QEMU/KVM.

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VmStartCommand) printInstanceInfo() {
	fmt.Print("\r")
	c.UI.Info("\r\nYour Trellis VM is ready to use!\r\n\r\n* Composer and WP-CLI commands need to be run on the virtual machine for any post-provision modifications.\r\n* You can SSH into the machine with 'trellis vm shell'\r\n* Then navigate to your WordPress sites at '/srv/www'")

	if runtime.GOOS == "windows" {
		projectName := filepath.Base(filepath.Dir(c.Trellis.Path))
		fmt.Print("\r")
		c.UI.Info(fmt.Sprintf("\r\nIMPORTANT -- Windows/WSL2 development workflow:\r\n  Your project has been copied to WSL2 at: /home/admin/%s\r\n  This is your working directory for editing files and using git.\r\n  Do NOT edit files on the Windows side -- they are only used during initial setup.\r\n\r\n  To open VS Code in the correct location, run:\r\n    trellis vm open", projectName))
	}
}
