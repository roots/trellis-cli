package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

func NewSshCommand(ui cli.Ui, trellis *trellis.Trellis) *SshCommand {
	c := &SshCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type SshCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	user    string
}

func (c *SshCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.user, "u", "", "User to connect as")
	c.flags.StringVar(&c.user, "user", "", "User to connect as")
}

func (c *SshCommand) Run(args []string) int {
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

	siteNameArg := ""
	if len(args) == 2 {
		siteNameArg = args[1]
	}
	siteName, siteNameErr := c.Trellis.FindSiteNameFromEnvironment(environment, siteNameArg)
	if siteNameErr != nil {
		c.UI.Error(siteNameErr.Error())
		return 1
	}

	sshHost := c.Trellis.SshHost(environment, siteName, c.user)

	ssh := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(c.UI),
	).Cmd("ssh", []string{sshHost})

	if err := ssh.Run(); err != nil {
		c.UI.Error(fmt.Sprintf("Error running ssh: %s", err))
		return 1
	}

	return 0
}

func (c *SshCommand) Synopsis() string {
	return "Connects to host via SSH"
}

func (c *SshCommand) Help() string {
	helpText := `
Usage: trellis ssh [options] ENVIRONMENT [SITE]

Connects to the main canonical host via SSH for the specified environment.

Connects to main production site host:

  $ trellis ssh production

Connects to non-main production site host:

  $ trellis ssh production mysite.com

Connects to production as web user:

  $ trellis ssh -u web production

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com)

Options:
  -u, --user  User to connect as
  -h, --help  Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *SshCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite(c.flags)
}

func (c *SshCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--user": complete.PredictNothing,
	}
}
