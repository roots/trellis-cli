package cmd

import (
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type DropletCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *DropletCommand) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *DropletCommand) Synopsis() string {
	return "Commands for DigitalOcean Droplets"
}

func (c *DropletCommand) Help() string {
	helpText := `
Usage: trellis droplet <subcommand> [<args>]
`

	return strings.TrimSpace(helpText)
}
