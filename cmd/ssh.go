package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
)

type SshCommand struct {
	UI              cli.Ui
	Trellis         *trellis.Trellis
	CommandExecutor CommandExecutor
}

func (c *SshCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	var environment string
	var siteName string
	var user string

	switch len(args) {
	case 0:
		c.UI.Output(c.Help())
		return 1
	case 1:
		environment = args[0]
	case 2:
		environment = args[0]
		siteName = args[1]
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 2, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	_, ok := c.Trellis.Environments[environment]
	if !ok {
		c.UI.Error(fmt.Sprintf("Error: %s is not a valid environment", environment))
		return 1
	}

	if siteName == "" {
		sites := c.Trellis.SiteNamesFromEnvironment(environment)
		siteName = sites[0]
	} else {
		site := c.Trellis.SiteFromEnvironmentAndName(environment, siteName)

		if site == nil {
			c.UI.Error(fmt.Sprintf("Error: %s is not a valid site", siteName))
			return 1
		}
	}

	host := c.Trellis.SiteFromEnvironmentAndName(environment, siteName).MainHost()

	if environment == "development" {
		user = "vagrant"
	} else {
		user = "admin"
	}

	host = fmt.Sprintf("%s@%s", user, host)

	ssh, _ := c.CommandExecutor.LookPath("ssh")
	sshArgs := []string{"ssh", host}
	env := os.Environ()
	execErr := c.CommandExecutor.Exec(ssh, sshArgs, env)

	if execErr != nil {
		c.UI.Error(fmt.Sprintf("Error running ssh: %s", execErr))
		return 1
	}

	return 0
}

func (c *SshCommand) Synopsis() string {
	return "Connects to host via SSH"
}

func (c *SshCommand) Help() string {
	helpText := `
Usage: trellis ssh [options] ENVIRONMENT SITE

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
	return c.Trellis.AutocompleteSite()
}

func (c *SshCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}
