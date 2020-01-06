package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
)

func NewRollbackCommand(ui cli.Ui, trellis *trellis.Trellis) *RollbackCommand {
	c := &RollbackCommand{UI: ui, Trellis: trellis, playbook: &Playbook{ui: ui}}
	c.init()
	return c
}

type RollbackCommand struct {
	UI       cli.Ui
	flags    *flag.FlagSet
	release  string
	Trellis  *trellis.Trellis
	playbook PlaybookRunner
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

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 1}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	environment := args[0]
	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	siteNameArg := c.flags.Arg(1)
	siteName, siteNameErr := c.Trellis.FindSiteNameFromEnvironment(environment, siteNameArg)
	if siteNameErr != nil {
		c.UI.Error(siteNameErr.Error())
		return 1
	}

	extraVars := fmt.Sprintf("env=%s site=%s", environment, siteName)

	if len(c.release) > 0 {
		extraVars = fmt.Sprintf("%s release=%s", extraVars, c.release)
	}

	c.playbook.SetRoot(c.Trellis.Path)

	if err := c.playbook.Run("rollback.yml", []string{"-e", extraVars}); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *RollbackCommand) Synopsis() string {
	return "Rollsback the last deploy of the site on the specified environment"
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
