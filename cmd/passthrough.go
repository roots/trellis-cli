package cmd

import (
	"fmt"
	"github.com/mitchellh/cli"
	"trellis-cli/plugin"
)

type PassthroughCommand struct {
	UI   cli.Ui
	Plugin plugin.Plugin
}

func (c *PassthroughCommand) Run(args []string) int {
	command := CommandExec(c.Plugin.Bin, args, c.UI)
	err := command.Run()

	if err != nil {
		c.UI.Error(fmt.Sprintf("Error running %s: %s", c.Plugin.Bin, err))
		return 1
	}

	return 0
}

func (c *PassthroughCommand) Synopsis() string {
	return c.Plugin.SynopsisText
}

func (c *PassthroughCommand) Help() string {
	return c.Plugin.HelpText
}
