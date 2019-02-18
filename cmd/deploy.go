package cmd

import (
	"fmt"
	"log"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
)

type DeployCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *DeployCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	var environment string
	var siteName string

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

		if len(sites) > 1 {
			c.UI.Error("Error: missing SITE argument\n")
			c.UI.Output(c.Help())
			return 1
		}

		siteName = sites[0]
	}

	deploy := execCommand("./bin/deploy.sh", environment, siteName)
	logCmd(deploy, c.UI, true)
	err := deploy.Run()

	if err != nil {
		log.Fatal(err)
	}

	return 0
}

func (c *DeployCommand) Synopsis() string {
	return "Deploys a site to the specified environment."
}

func (c *DeployCommand) Help() string {
	helpText := `
Usage: trellis deploy [options] ENVIRONMENT SITE

Deploys a site to the specified environment.

See https://roots.io/trellis/docs/deploys/ for more information on deploys with Trellis.

Deploy a site to production:

  $ trellis deploy production example.com

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com)

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}

func (c *DeployCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite()
}

func (c *DeployCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}
