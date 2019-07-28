package cmd

import (
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type ValetCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *ValetCommand) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *ValetCommand) Synopsis() string {
	return "Commands for Laravel Valet"
}

func (c *ValetCommand) Help() string {
	helpText := `
Usage: trellis valet <subcommand> [<args>]
`

	return strings.TrimSpace(helpText)
}
