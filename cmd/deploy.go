package cmd

import (
	"fmt"
	"log"
	"os/exec"
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
	c.Trellis.EnforceValid(c.UI)

	var environment string
	var siteName string

	switch len(args) {
	case 0:
		c.UI.Output(c.Help())
		return 1
	case 1:
		c.UI.Error("Missing SITE argument\n")
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

	deploy := exec.Command("./bin/deploy.sh", environment, siteName)
	logCmd(deploy, true)
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
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}

func (c *DeployCommand) AutocompleteArgs() complete.Predictor {
	return c.PredictSite()
}

func (c *DeployCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}

func (c *DeployCommand) PredictSite() complete.PredictFunc {
	return func(args complete.Args) []string {
		switch len(args.Completed) {
		case 1:
			return c.Trellis.EnvironmentNames()
		case 2:
			return c.Trellis.SiteNamesFromEnvironment(args.LastCompleted)
		default:
			return []string{}
		}
	}
}
