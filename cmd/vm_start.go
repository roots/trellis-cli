package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/hostagent"
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

	if c.Trellis.CliConfig.VmHostsResolver == "hostagent" && !hostagent.Running() {
		if err := c.hostagentInstall(); err != nil {
			return 1
		}
	}

	instance := manager.NewInstance(siteName)
	_, ok := manager.GetInstance(siteName)

	if ok {
		if err := instance.Hydrate(true); err != nil {
			c.UI.Error("Error getting VM info. This is a trellis-cli bug.")
			c.UI.Error(err.Error())
			return 1
		}

		if instance.Running() {
			c.UI.Info(fmt.Sprintf("%s VM already running", color.GreenString("[✓]")))
		} else {
			if err := instance.Start(c.UI); err != nil {
				c.UI.Error("Error starting virtual machine.")
				c.UI.Error(err.Error())
				return 1
			}
		}

		if err := manager.HostsResolver.AddHosts(instance.Name, &instance); err != nil {
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

	if err := manager.HostsResolver.AddHosts(instance.Name, &instance); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err = instance.CreateInventoryFile(); err != nil {
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

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VmStartCommand) hostagentInstall() error {
	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Checking hostagent requirements",
			FailMessage: "hostagent requirements not met",
		},
	)

	spinner.Start()
	portsInUse := hostagent.PortsInUse()

	if len(portsInUse) > 0 {
		spinner.StopFail()
		c.UI.Error("The hostagent runs a reverse HTTP proxy and a local DNS resolver and requires a few ports to be free. The following ports are already in use by another service on your machine:")

		for _, port := range portsInUse {
			c.UI.Error(fmt.Sprintf("%s %d", port.Protocol, port.Number))
		}

		c.UI.Error("\nUsing the `lsof` command will let you know what process is using the port:")
		for _, port := range portsInUse {
			c.UI.Error(fmt.Sprintf("=> sudo lsof -nP -iTCP:%d -sTCP:LISTEN", port.Number))
		}

		return fmt.Errorf("install failed")
	}

	spinner.Stop()

	if hostagent.Installed() {
		if err := c.runHostagent(); err != nil {
			return err
		}
	} else {
		c.UI.Info(fmt.Sprintf("%s hostagent not installed", color.RedString("[✘]")))
		c.UI.Info("\ntrellis-cli hostagent needs to be installed on your host machine for VM integration. The hostagent is a service that runs in the background with a reverse HTTP proxy and a local DNS resolver.")
		c.UI.Info("The DNS resolver will resolve queries for the *.test domain and always respond with 127.0.0.1. Using a DNS resolver means your /etc/hosts does not need to be updated.")
		c.UI.Info("The HTTP server runs on port 80 and proxies requests from site hosts to the VM's forward port. Example: example.test:80 -> 127.0.0.1:63208")
		c.UI.Info("\nTwo files are created as part of the installation:")
		c.UI.Info(" 1. " + hostagent.PlistPath())
		c.UI.Info(" 2. " + hostagent.ResolverPath())
		c.UI.Info("\nNote: sudo is needed to create the resolver file. The hostagent service will run under your user account, not as root.")

		if err := hostagent.Install(); err != nil {
			c.UI.Error("Error installing trellis-cli hostagent service.")
			c.UI.Error(err.Error())
			return err
		}
	}

	return nil
}

func (c *VmStartCommand) runHostagent() error {
	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Starting hostagent",
			FailMessage: "Hostagent could not start",
		},
	)

	spinner.Start()
	if err := hostagent.RunServer(); err != nil {
		spinner.StopFail()
		return err
	}

	spinner.Stop()
	return nil
}
