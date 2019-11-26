package cmd

import (
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type DBCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *DBCommand) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *DBCommand) Synopsis() string {
	return "Commands for database management"
}

func (c *DBCommand) Help() string {
	helpText := `
Usage: trellis db <subcommand> [<args>]
`

	return strings.TrimSpace(helpText)
}
