package cmd

import (
	"flag"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

const VagrantInventoryFilePath string = ".vagrant/provisioners/ansible/inventory/vagrant_ansible_inventory"
const LimaInventoryFilePath string = ".trellis/lima/inventory"

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
	verbose   bool
}

func (c *ProvisionCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.extraVars, "extra-vars", "", "Additional variables which are passed through to Ansible as 'extra-vars'")
	c.flags.StringVar(&c.tags, "tags", "", "only run roles and tasks tagged with these values")
	c.flags.BoolVar(&c.verbose, "verbose", false, "Enable Ansible's verbose mode")
}

func (c *ProvisionCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

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

	galaxyInstallCommand := &GalaxyInstallCommand{c.UI, c.Trellis}
	galaxyInstallCommand.Run([]string{})

	vars := "env=" + environment
	if c.extraVars != "" {
		vars = strings.Join([]string{vars, c.extraVars}, " ")
	}

	playbookArgs := []string{"-e", vars}
	if c.tags != "" {
		playbookArgs = append(playbookArgs, "--tags", c.tags)
	}

	if c.verbose {
		playbookArgs = append(playbookArgs, "-vvvv")
	}

	var playbookFile string = "server.yml"

	if environment == "development" {
		playbookFile = "dev.yml"
		devInventoryFile := findDevHostsFile(c.Trellis.Path)

		if devInventoryFile != "" {
			playbookArgs = append(playbookArgs, "--inventory-file", devInventoryFile)
		}
	}

	provision := command.WithOptions(
		command.WithUiOutput(c.UI),
		command.WithLogging(c.UI),
	).Cmd("ansible-playbook", append([]string{playbookFile}, playbookArgs...))

	if err := provision.Run(); err != nil {
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
See https://docs.roots.io/trellis/master/remote-server-setup/#provision for more details.

Provision the production environment:

  $ trellis provision production

Multiple args within quotes, separated by space:

  $ trellis provision --extra-vars "key1=value key2=value" --tags "users mail" production

Provision the production environment but only run the 'users' role:

  $ trellis provision --tags users production

Provision and provide extra vars to Ansible:

  $ trellis provision --extra-vars key=value production

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  
Options:
      --extra-vars  (multiple) set additional variables as key=value or YAML/JSON, if filename prepend with @
      --tags        (multiple) only run roles and tasks tagged with these values
      --verbose     Enable Ansible's verbose mode
  -h, --help        show this help
`

	return strings.TrimSpace(helpText)
}

func (c *ProvisionCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment(c.flags)
}

func (c *ProvisionCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--extra-vars": complete.PredictNothing,
		"--tags":       complete.PredictNothing,
		"--verbose":    complete.PredictNothing,
	}
}

func findDevHostsFile(path string) string {
	if _, limaInventoryErr := os.Stat(filepath.Join(path, LimaInventoryFilePath)); limaInventoryErr == nil {
		return LimaInventoryFilePath
	}

	if _, vagrantInventoryErr := os.Stat(filepath.Join(path, VagrantInventoryFilePath)); vagrantInventoryErr == nil {
		return VagrantInventoryFilePath
	}

	return ""
}
