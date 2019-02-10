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

func NewProvisionCommand(ui cli.Ui, trellis *trellis.Trellis) *ProvisionCommand {
	c := &ProvisionCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type ProvisionCommand struct {
	UI        cli.Ui
	flags     *flag.FlagSet
	extraVars string
	tags      string
	Trellis   *trellis.Trellis
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

	var environment string

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	switch len(args) {
	case 0:
		c.UI.Error("Error: missing ENVIRONMENT argument\n")
		c.UI.Output(c.Help())
		return 1
	case 1:
		environment = args[0]
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 1, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	extraVars := fmt.Sprintf("env=%s", environment)

	if len(c.extraVars) > 0 {
		fmt.Println(c.extraVars)
		extraVars = strings.Join([]string{extraVars, c.extraVars}, " ")
	}

	playbookArgs := []string{"server.yml", "-e", extraVars}

	if len(c.tags) > 0 {
		playbookArgs = append(playbookArgs, "--tags", c.tags)
	}

	playbook := execCommand("ansible-playbook", playbookArgs...)
	logCmd(playbook, c.UI, true)
	err := playbook.Run()

	if err != nil {
		log.Fatal(err)
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
  --extra-vars     (multiple) set additional variables as key=value or YAML/JSON, if filename prepend with @
  --tags           (multiple) only run roles and tasks tagged with these values
  -h, --help       show this help
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
