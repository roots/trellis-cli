package cmd

import (
	"flag"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
)

func NewProvisionCommand(ui cli.Ui, trellis *trellis.Trellis) *ProvisionCommand {
	c := &ProvisionCommand{UI: ui, Trellis: trellis, playbook: &Playbook{}}
	c.init()
	return c
}

type ProvisionCommand struct {
	UI        cli.Ui
	flags     *flag.FlagSet
	extraVars string
	tags      string
	Trellis   *trellis.Trellis
	playbook  PlaybookRunner
}

func (c *ProvisionCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.tags, "tags", "", "only run roles and tasks tagged with these values")
	c.flags.StringVar(&c.extraVars, "extra-vars", "", "Additional variables which are passed through to Ansible as 'extra-vars'")
}

func (c *ProvisionCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 0}
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

	c.playbook.SetRoot(c.Trellis.Path)

	vars := "env=" + environment
	if c.extraVars != "" {
		vars = strings.Join([]string{vars, c.extraVars}, " ")
	}

	playbookArgs := []string{"-e", vars}
	if c.tags != "" {
		playbookArgs = append(playbookArgs, "--tags", c.tags)
	}

	if err := c.playbook.Run("server.yml", playbookArgs, c.UI); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *ProvisionCommand) Synopsis() string {
	return "Provisions the specified environment"
}

func (c *ProvisionCommand) Help() string {
	helpText := `
Usage: trellis provision [options] ENVIRONMENT

Provisions a server on the specified environment.

This is considered a safe operation and can be re-run on existing servers to apply new configuration changes.
See https://roots.io/trellis/docs/remote-server-setup/#provision for more details.

Provision the production environment:

  $ trellis provision production

Provision the production environment but only run the 'users' role:

  $ trellis provision --tags users production

Provision and provide extra vars to Ansible:

  $ trellis provision --extra-vars key=value production

Multiple vars should be quoted:

  $ trellis provision --extra-vars "key1=value key2=value" production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
      --extra-vars  (multiple) set additional variables as key=value or YAML/JSON, if filename prepend with @
      --tags        (multiple) only run roles and tasks tagged with these values
  -h, --help        show this help
`

	return strings.TrimSpace(helpText)
}

func (c *ProvisionCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment()
}

func (c *ProvisionCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--extra-vars": complete.PredictNothing,
		"--tags":       complete.PredictNothing,
	}
}
