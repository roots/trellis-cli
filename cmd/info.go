package cmd

import (
	"fmt"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

type InfoCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *InfoCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	for name, config := range c.Trellis.Environments {
		var siteNames []string

		for name := range config.WordPressSites {
			siteNames = append(siteNames, name)
		}

		c.UI.Info(fmt.Sprintf("%s => %s", name, strings.Join(siteNames, ", ")))
	}
	return 0
}

func (c *InfoCommand) Synopsis() string {
	return "Displays information about this Trellis project"
}

func (c *InfoCommand) Help() string {
	helpText := `
Usage: trellis info [options]

Displays information about this Trellis project

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
