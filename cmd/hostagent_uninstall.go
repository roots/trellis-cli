package cmd

import (
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/hostagent"
	"github.com/roots/trellis-cli/trellis"
)

type HostagentUninstallCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *HostagentUninstallCommand) Run(args []string) int {
	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	if !hostagent.Installed() {
		c.UI.Info("hostagent not installed. Nothing more to uninstall.")
		return 0
	}

	if err := hostagent.Uninstall(); err != nil {
		c.UI.Error("Error uninstalling hostagent:")
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *HostagentUninstallCommand) Synopsis() string {
	return "Uninstalls the trellis-cli hostagent"
}

func (c *HostagentUninstallCommand) Help() string {
	helpText := `
Usage: trellis hostagent uninstall [options]

Uninstalls the trellis-cli hostagent
  
Options:
  -h, --help Show this help
`

	return strings.TrimSpace(helpText)
}
