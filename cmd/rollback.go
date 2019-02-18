package cmd

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
)

func NewRollbackCommand(ui cli.Ui, trellis *trellis.Trellis) *RollbackCommand {
	c := &RollbackCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type RollbackCommand struct {
	UI      cli.Ui
	flags   *flag.FlagSet
	release string
	Trellis *trellis.Trellis
}

func (c *RollbackCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.release, "release", "", "Release to rollback instead of latest one")
}

func (c *RollbackCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	var environment string
	var siteName string

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	switch len(args) {
	case 0:
		c.UI.Output(c.Help())
		return 1
	case 1:
		c.UI.Error("Error: missing SITE argument\n")
		c.UI.Output(c.Help())
		return 1
	case 2:
		environment = args[0]
		siteName = args[1]
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 2, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	extraVars := fmt.Sprintf("env=%s site=%s", environment, siteName)

	if len(c.release) > 0 {
		extraVars = fmt.Sprintf("%s release=%s", extraVars, c.release)
	}

	playbookArgs := []string{"rollback.yml", "-e", extraVars}
	playbook := execCommand("ansible-playbook", playbookArgs...)
	logCmd(playbook, c.UI, true)
	err := playbook.Run()

	if err != nil {
		log.Fatal(err)
	}

	return 0
}

func (c *RollbackCommand) Synopsis() string {
	return "Rollsback the last deploy of the site on the specified environment."
}

func (c *RollbackCommand) Help() string {
	helpText := `
Usage: trellis rollback [options] ENVIRONMENT SITE

Rollsback the last deploy for the site specified.

Rollback the latest deploy:

  $ trellis rollback production example.com

Rollback a specific release:

  $ trellis rollback --release=12345678901234 production example.com

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com)

Options:
      --release  Name of release to rollback instead of latest
  -h, --help     show this help
`

	return strings.TrimSpace(helpText)
}

func (c *RollbackCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite()
}

func (c *RollbackCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--release": complete.PredictNothing,
	}
}
