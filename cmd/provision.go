package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/ansible"
	"github.com/roots/trellis-cli/trellis"
)

func NewProvisionCommand(ui cli.Ui, trellis *trellis.Trellis) *ProvisionCommand {
	c := &ProvisionCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type ProvisionCommand struct {
	UI           cli.Ui
	flags        *flag.FlagSet
	extraVars    string
	interactive  bool
	playbookName string
	tags         string
	Trellis      *trellis.Trellis
	verbose      bool
}

func (c *ProvisionCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.extraVars, "extra-vars", "", "Additional variables which are passed through to Ansible as 'extra-vars'")
	c.flags.StringVar(&c.tags, "tags", "", "only run roles and tasks tagged with these values")
	c.flags.BoolVar(&c.verbose, "verbose", false, "Enable Ansible's verbose mode")
	c.flags.BoolVar(&c.interactive, "interactive", false, "Enable interactive mode to select tags to provision")
	c.flags.BoolVar(&c.interactive, "i", false, "Enable interactive mode to select tags to provision")
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

	c.playbookName = "server.yml"

	if environment == "development" {
		c.playbookName = "dev.yml"
		os.Setenv("ANSIBLE_HOST_KEY_CHECKING", "false")
	}

	playbook := ansible.Playbook{
		Name:    c.playbookName,
		Env:     environment,
		Verbose: c.verbose,
	}

	if c.extraVars != "" {
		playbook.AddExtraVars(c.extraVars)
	}

	if c.tags != "" {
		if c.interactive {
			c.UI.Error("Error: --interactive and --tags cannot be used together. Please use one or the other.")
			return 1
		}

		playbook.AddArg("--tags", c.tags)
	}

	if c.interactive {
		_, err := exec.LookPath("fzf")
		if err != nil {
			c.UI.Error("Error: `fzf` command found. fzf is required to use interactive mode.")
			return 1
		}

		tags, err := c.getTags()
		if err != nil {
			c.UI.Error("Error getting Ansible playbook tags.")
			c.UI.Error(err.Error())
			c.UI.Error("This is probably a trellis-cli bug. Please open an issue at: https://github.com/roots/trellis-cli")
			return 1
		}

		selectedTags, err := c.selectedTagsFromFzf(tags)
		if err != nil {
			return 1
		}

		playbook.AddArg("--tags", strings.Join(selectedTags, ","))
	}

	if environment == "development" {
		playbook.SetInventory(findDevInventory(c.Trellis, c.UI))
	}

	galaxyInstallCommand := &GalaxyInstallCommand{c.UI, c.Trellis}
	galaxyInstallCommand.Run([]string{})

	provision := command.WithOptions(
		command.WithUiOutput(c.UI),
		command.WithLogging(c.UI),
	).Cmd("ansible-playbook", playbook.CmdArgs())

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
See https://roots.io/trellis/docs/remote-server-setup/#provision for more details.

Provision the production environment:

  $ trellis provision production

Multiple args within quotes, separated by space:

  $ trellis provision --extra-vars "key1=value key2=value" --tags "users mail" production

Provision the production environment but only run the 'users' role:

  $ trellis provision --tags users production

Provision and provide extra vars to Ansible:

  $ trellis provision --extra-vars key=value production

Provision using interactive mode to select tags:

  $ trellis provision -i production

Arguments:
  ENVIRONMENT Name of environment (ie: production)
  
Options:
      --extra-vars  (multiple) Set additional variables as key=value or YAML/JSON, if filename prepend with @
  -i, --interactive Enter interactive mode to select tags to provision (requires fzf)
      --tags        (multiple) Only run roles and tasks tagged with these values
      --verbose     Enable Ansible's verbose mode
  -h, --help        Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *ProvisionCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment(c.flags)
}

func (c *ProvisionCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-i":            complete.PredictNothing,
		"--interactive": complete.PredictNothing,
		"--extra-vars":  complete.PredictNothing,
		"--tags":        complete.PredictNothing,
		"--verbose":     complete.PredictNothing,
	}
}

func (c *ProvisionCommand) getTags() ([]string, error) {
	tagsPlaybook := ansible.Playbook{
		Name: c.playbookName,
		Env:  c.flags.Arg(0),
		Args: []string{"--list-tags"},
	}

	tagsProvision := command.WithOptions(
		command.WithUiOutput(c.UI),
	).Cmd("ansible-playbook", tagsPlaybook.CmdArgs())

	output := &bytes.Buffer{}
	tagsProvision.Stdout = output

	if err := tagsProvision.Run(); err != nil {
		return nil, err
	}

	tags := ansible.ParseTags(output.String())

	return tags, nil
}

func (c *ProvisionCommand) selectedTagsFromFzf(tags []string) ([]string, error) {
	output := &bytes.Buffer{}
	input := strings.NewReader(strings.Join(tags, "\n"))

	previewCmd := fmt.Sprintf("trellis exec ansible-playbook %s --list-tasks --tags {}", c.playbookName)

	fzf := command.WithOptions(command.WithTermOutput()).Cmd(
		"fzf",
		[]string{
			"-m",
			"--height", "50%",
			"--reverse",
			"--border",
			"--border-label", "Select tags to provision (use TAB to select multiple tags)",
			"--border-label-pos", "5",
			"--preview", previewCmd,
			"--preview-label", "Tasks for tag",
		},
	)
	fzf.Stdin = input
	fzf.Stdout = output

	err := fzf.Run()
	if err != nil {
		return nil, err
	}

	selectedTags := strings.Split(strings.TrimSpace(output.String()), "\n")

	return selectedTags, nil
}
