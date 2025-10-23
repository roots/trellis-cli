package cmd

import (
	"github.com/hashicorp/cli"
)

type NamespaceCommand struct {
	SynopsisText string
	HelpText     string
}

func (c *NamespaceCommand) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *NamespaceCommand) Synopsis() string {
	return c.SynopsisText
}

func (c *NamespaceCommand) Help() string {
	return c.HelpText
}
