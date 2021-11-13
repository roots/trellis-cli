package cmd

import (
	"flag"
	"fmt"
	"log"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/trellis"
)

func NewDeployCommand(ui cli.Ui, trellis *trellis.Trellis) *DeployCommand {
	c := &DeployCommand{UI: ui, Trellis: trellis, playbook: &Playbook{ui: ui}}
	c.init()
	return c
}

type DeployCommand struct {
	UI        cli.Ui
	flags     *flag.FlagSet
	branch    string
	extraVars string
	Trellis   *trellis.Trellis
	playbook  PlaybookRunner
}

func (c *DeployCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.branch, "branch", "", "Optional git branch to deploy which overrides the branch set in your site config (default: master)")
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

	vars := []string{
		fmt.Sprintf("env=%s", environment),
		fmt.Sprintf("site=%s", siteName),
	}

	if c.branch != "" {
		vars = append(vars, fmt.Sprintf("branch=%s", c.branch))
	}

	if c.extraVars != "" {
		vars = append(vars, c.extraVars)
	}

	extraVars := strings.Join(vars, " ")

	c.playbook.SetRoot(c.Trellis.Path)

	if err := c.playbook.Run("deploy.yml", []string{"-e", extraVars}); err != nil {
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

Deploy a site to staging with a dfferent git branch:

  $ trellis deploy --branch=feature-123 production example.com

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com)

Options:
      --branch      Optional git branch to deploy which overrides the branch set in your site config (default: master)
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
		"--branch":     complete.PredictNothing,
		"--extra-vars": complete.PredictNothing,
	}
}
