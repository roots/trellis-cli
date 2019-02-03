package cmd

import (
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type VaultCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *VaultCommand) Run(args []string) int {
	c.UI.Output(c.Help())

	return 0
}

func (c *VaultCommand) Synopsis() string {
	return "Commands for Ansible Vault"
}

func (c *VaultCommand) Help() string {
	helpText := `
Usage: trellis vault <subcommand> [<args>]
`

	return strings.TrimSpace(helpText)
}
