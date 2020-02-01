package cmd

import (
	"github.com/mitchellh/cli"
)

type DBCommand struct{}

func (c *DBCommand) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *DBCommand) Synopsis() string {
	return "Commands for database management"
}

func (c *DBCommand) Help() string {
	return "Usage: trellis db <subcommand> [<args>]"
}
