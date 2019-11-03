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

func NewDeployCommand(ui cli.Ui, trellis *trellis.Trellis) *DeployCommand {
	c := &DeployCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type DeployCommand struct {
	UI        cli.Ui
	flags     *flag.FlagSet
	extraVars string
	Trellis   *trellis.Trellis
}

func (c *DeployCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.extraVars, "extra-vars", "", "Additional variables which are passed through to Ansible as 'extra-vars'")
}

func (c *DeployCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	argCountErr := validateArgumentCount(args, 1, 1)
	if argCountErr != nil {
		c.UI.Error(argCountErr.Error())
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

	vars := []string{
		fmt.Sprintf("env=%s", environment),
		fmt.Sprintf("site=%s", siteName),
	}

	if c.extraVars != "" {
		vars = append(vars, c.extraVars)
	}

	extraVars := fmt.Sprintf("\"%s\"", strings.Join(vars, " "))

	playbookArgs := []string{"deploy.yml", "-e", extraVars}
	deploy := execCommand("ansible-playbook", playbookArgs...)
	logCmd(deploy, c.UI, true)
	err := deploy.Run()

	if err != nil {
		log.Fatal(err)
	}

	return 0
}

func (c *DeployCommand) Synopsis() string {
	return "Deploys a site to the specified environment"
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
      --extra-vars  (multiple) set additional variables as key=value or YAML/JSON, if filename prepend with @
  -h, --help        show this help
`

	return strings.TrimSpace(helpText)
}

func (c *DeployCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite()
}

func (c *DeployCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--extra-vars": complete.PredictNothing,
	}
}
