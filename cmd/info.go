package cmd

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
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

	for name, sites := range c.Trellis.Environments {
		var siteNames []string

		for _, site := range sites {
			siteNames = append(siteNames, site.Name)
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
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}
