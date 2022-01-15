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

func NewDeployCommand(ui cli.Ui, trellis *trellis.Trellis) *DeployCommand {
	c := &DeployCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type DeployCommand struct {
	UI        cli.Ui
	flags     *flag.FlagSet
	branch    string
	extraVars string
	Trellis   *trellis.Trellis
	verbose   bool
}

func (c *DeployCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.branch, "branch", "", "Optional git branch to deploy which overrides the branch set in your site config (default: master)")
	c.flags.StringVar(&c.extraVars, "extra-vars", "", "Additional variables which are passed through to Ansible as 'extra-vars'")
	c.flags.BoolVar(&c.verbose, "verbose", false, "Enable Ansible's verbose mode")
}

func (c *DeployCommand) Run(args []string) int {
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

	vars := []string{
		fmt.Sprintf("env=%s", environment),
		fmt.Sprintf("site=%s", siteName),
	}

	if c.verbose {
		vars = append(vars, "-vvvv")
	}

	if c.branch != "" {
		vars = append(vars, fmt.Sprintf("branch=%s", c.branch))
	}

	if c.extraVars != "" {
		vars = append(vars, c.extraVars)
	}

	extraVars := strings.Join(vars, " ")

	deploy := command.WithOptions(
		command.WithUiOutput(c.UI),
		command.WithLogging(c.UI),
	).Cmd("ansible-playbook", []string{"deploy.yml", "-e", extraVars})

	if err := deploy.Run(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *DeployCommand) Synopsis() string {
	return "Deploys a site to the specified environment"
}

func (c *DeployCommand) Help() string {
	helpText := `
Usage: trellis deploy [options] ENVIRONMENT [SITE]

Deploys a site to the specified environment.

See https://docs.roots.io/trellis/master/deployments/ for more information on deploys with Trellis.

Deploy the default site to production:

  $ trellis deploy production

Deploy example.com site to production:

  $ trellis deploy production example.com

Deploy a site to staging with a different git branch:

  $ trellis deploy --branch=feature-123 production example.com

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  SITE        Name of the site (ie: example.com)

Options:
      --branch      Optional git branch to deploy which overrides the branch set in your site config (default: master)
      --extra-vars  (multiple) set additional variables as key=value or YAML/JSON, if filename prepend with @
      --verbose     Enable Ansible's verbose mode
  -h, --help        show this help
`

	return strings.TrimSpace(helpText)
}

func (c *DeployCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteSite(c.flags)
}

func (c *DeployCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--branch":     complete.PredictNothing,
		"--extra-vars": complete.PredictNothing,
		"--verbose":    complete.PredictNothing,
	}
}
