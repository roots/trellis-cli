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

	siteName := ""
	if len(args) == 2 {
		siteName = args[1]
	}

	_, ok := c.Trellis.Environments[environment]
	if !ok {
		c.UI.Error(fmt.Sprintf("Error: %s is not a valid environment", environment))
		return 1
	}

	if siteName == "" {
		sites := c.Trellis.SiteNamesFromEnvironment(environment)

		if len(sites) > 1 {
			c.UI.Error("Error: missing SITE argument\n")
			c.UI.Output(c.Help())
			return 1
		}

		siteName = sites[0]
	} else {
		site := c.Trellis.SiteFromEnvironmentAndName(environment, siteName)

		if site == nil {
			c.UI.Error(fmt.Sprintf("Error: %s is not a valid site", siteName))
			return 1
		}
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
