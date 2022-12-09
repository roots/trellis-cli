package cmd

import (
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/lima"
	"github.com/roots/trellis-cli/trellis"
)

type VmShellCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *VmShellCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 1}
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

	sites := c.Trellis.Environments["development"].WordPressSites
	manager, err := lima.NewManager(c.Trellis.ConfigPath(), sites)

	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	instance, ok := manager.GetInstance(siteName)

	if !ok {
		c.UI.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return 0
	}

	if err := instance.Hydrate(); err != nil {
		c.UI.Error("Error getting VM info. This is a trellis-cli bug.")
		c.UI.Error(err.Error())
		return 1
	}

	if instance.Stopped() {
		c.UI.Info("VM is stopped. Run `trellis vm start` to start it.")
	} else {
		if err := instance.Shell(args); err != nil {
			c.UI.Error(err.Error())
			return 1
		}
	}

	return 0
}

func (c *VmShellCommand) Synopsis() string {
	return "Execute shell in the VM"
}

func (c *VmShellCommand) Help() string {
	helpText := `
Usage: trellis vm shell [options] [COMMAND]

Execute shell in the VM.

Run an optional command from the VM shell:

  $ trellis vm shell whoami

Arguments:
  COMMAND  Command to execute

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}
