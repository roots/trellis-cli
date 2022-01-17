package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type SshCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *SshCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

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

	host := c.Trellis.SiteFromEnvironmentAndName(environment, siteName).MainHost()

	var user string
	if environment == "development" {
		user = "vagrant"
	} else {
		user = "admin"
	}

	host = fmt.Sprintf("%s@%s", user, host)

	ssh := command.WithOptions(command.WithTermOutput(), command.WithLogging(c.UI)).Cmd("ssh", []string{host})
	err := ssh.Run()

	if err != nil {
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

  Note: production always connects with 'admin' user

Connects to non-main production site host:

  $ trellis ssh production mysite.com

Connects to main development site host:

  $ trellis ssh development

  Note: development always connects with 'vagrant' user

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com)

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}

func (c *SshCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite(flag.NewFlagSet("", flag.ContinueOnError))
}

func (c *SshCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}
