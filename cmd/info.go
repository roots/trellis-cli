package cmd

import (
	"fmt"
	"strings"

	"trellis-cli/trellis"

	"github.com/mitchellh/cli"
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

	for name, config := range c.Trellis.Environments {
		var siteNames []string

		for name, _ := range config.WordPressSites {
			siteNames = append(siteNames, name)
		}

		c.UI.Info(fmt.Sprintf("☁️ %s => %s", name, strings.Join(siteNames, ", ")))
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
