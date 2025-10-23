package cmd

import (
	"flag"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/ansible"
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

	playbook := ansible.Playbook{
		Name:    "deploy.yml",
		Env:     environment,
		Verbose: c.verbose,
		ExtraVars: map[string]string{
			"site": siteName,
		},
	}

	if environment == "development" {
		if !c.Trellis.CliConfig.AllowDevelopmentDeploys {
			c.UI.Error(`
  Error: deploying to the development environment is not supported by default.

  Most local development environments handle file sharing automatically.
  Local site files are automatically synced/shared to the VM so there's no need to manually deploy a site.

  See https://roots.io/trellis/docs/local-development/ for more details.

  If you're using a non-standard development setup (such as a remote cloud environment) and want to deploy,
  you can disable this check by setting the following config value in your CLI config:

    allow_development_deploys: true
      `)

			return 1
		}

		playbook.SetInventory(findDevInventory(c.Trellis, c.UI))
	}

	if c.branch != "" {
		playbook.AddExtraVar("branch", c.branch)
	}

	if c.extraVars != "" {
		playbook.AddExtraVars(c.extraVars)
	}

	deploy := command.WithOptions(
		command.WithUiOutput(c.UI),
		command.WithLogging(c.UI),
	).Cmd("ansible-playbook", playbook.CmdArgs())

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

See https://roots.io/trellis/docs/deployments/ for more information on deploys with Trellis.

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
