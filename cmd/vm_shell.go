package cmd

import (
	"strings"

	"github.com/mitchellh/cli"
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

	siteName, _, err := c.Trellis.MainSiteFromEnvironment("development")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	manager, err := newVmManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	if err := manager.OpenShell(siteName, args); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *VmShellCommand) Synopsis() string {
	return "Executes shell in the VM"
}

func (c *VmShellCommand) Help() string {
	helpText := `
Usage: trellis vm shell [options] [COMMAND]

Executes shell in the development virtual machine.

Run an optional command from the VM shell:

  $ trellis vm shell whoami

Arguments:
  COMMAND  Command to execute

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}
