package cmd

import (
	"flag"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/ansible"
	"github.com/roots/trellis-cli/trellis"
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
	verbose bool
}

func (c *RollbackCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.release, "release", "", "Release to rollback instead of latest one")
	c.flags.BoolVar(&c.verbose, "verbose", false, "Enable Ansible's verbose mode")
}

func (c *RollbackCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

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

	playbook := ansible.Playbook{
		Name:    "rollback.yml",
		Env:     environment,
		Verbose: c.verbose,
		ExtraVars: map[string]string{
			"site": siteName,
		},
	}

	if c.release != "" {
		playbook.AddExtraVar("release", c.release)
	}

	rollback := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(c.UI),
	).Cmd("ansible-playbook", playbook.CmdArgs())

	if err := rollback.Run(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *RollbackCommand) Synopsis() string {
	return "Rollback the last deploy of the site on the specified environment"
}

func (c *RollbackCommand) Help() string {
	helpText := `
Usage: trellis rollback [options] ENVIRONMENT [SITE]

Performs a rollback (revert) of the last deploy for the site specified.

Rollback the latest deploy on the default site:

  $ trellis rollback production

Rollback the latest deploy for a specific site:

  $ trellis rollback production example.com

Rollback a specific release:

  $ trellis rollback --release=12345678901234 production example.com

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com)

Options:
      --release  Name of release to rollback instead of latest
      --verbose  Enable Ansible's verbose mode
  -h, --help     show this help
`

	return strings.TrimSpace(helpText)
}

func (c *RollbackCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite(c.flags)
}

func (c *RollbackCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--release": complete.PredictNothing,
		"--verbose": complete.PredictNothing,
	}
}
