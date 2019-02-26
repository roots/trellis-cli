package cmd

import (
	"flag"
	"fmt"
	"strings"

	"trellis-cli/trellis"

	"github.com/mitchellh/cli"
)

type HaltCommand struct {
	UI          cli.Ui
	Trellis     *trellis.Trellis
	flags       *flag.FlagSet
	noGalaxy    bool
	noProvision bool
}

func NewHaltCommand(ui cli.Ui, trellis *trellis.Trellis) *HaltCommand {
	c := &HaltCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *HaltCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *HaltCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	switch len(args) {
	case 0:
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 0, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	vagrantArgs := []string{"halt"}

	vagrantHalt := execCommand("vagrant", vagrantArgs...)

	logCmd(vagrantHalt, c.UI, true)
	err := vagrantHalt.Run()
	if err != nil {
		return 1
	}

	return 0
}

func (c *HaltCommand) Synopsis() string {
	return "Starts and provisions the Vagrant environment by running 'vagrant halt'"
}

func (c *HaltCommand) Help() string {
	helpText := `
Usage: trellis halt [options]

Stops the Vagrant environment by running 'vagrant halt'.

Stop Vagrant VM:

  $ trellis halt

Options:
  -h, --help         show this help
`

	return strings.TrimSpace(helpText)
}
