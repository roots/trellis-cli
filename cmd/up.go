package cmd

import (
	"flag"
	"strings"
	"os"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
)

type UpCommand struct {
	UI          cli.Ui
	Trellis     *trellis.Trellis
	flags       *flag.FlagSet
	withGalaxy  bool
	noProvision bool
}

func NewUpCommand(ui cli.Ui, trellis *trellis.Trellis) *UpCommand {
	c := &UpCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *UpCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.withGalaxy, "with-galaxy", false, "Run Ansible Galaxy install")
	c.flags.BoolVar(&c.noProvision, "no-provision", false, "Skip provisioning")
}

func (c *UpCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	vagrantArgs := []string{"up"}

	if c.noProvision {
		vagrantArgs = append(vagrantArgs, "--no-provision")
	}

	vagrantUp := execCommandWithOutput("vagrant", vagrantArgs, c.UI)

	env := os.Environ()
	// To allow mockExecCommand injects its environment variables.
	if vagrantUp.Env != nil {
		env = vagrantUp.Env
	}

	if !c.withGalaxy {
		vagrantUp.Env = append(env, "SKIP_GALAXY=true")
	}

	err := vagrantUp.Run()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *UpCommand) Synopsis() string {
	return "Starts and provisions the Vagrant environment by running 'vagrant up'"
}

func (c *UpCommand) Help() string {
	helpText := `
Usage: trellis up [options]

Starts and provisions the Vagrant environment by running 'vagrant up'.

Start Vagrant VM:

  $ trellis up

Start VM without provisioning:

  $ trellis up --no-provision

Start VM and auto-run Galaxy install:

  $ trellis up --with-galaxy

Options:
      --no-provision (default: false) Skip provisioning
      --with-galaxy  (default: false) Run Ansible Galaxy install
  -h, --help         show this help
`

	return strings.TrimSpace(helpText)
}

func (c *UpCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *UpCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--no-provision": complete.PredictNothing,
		"--with-galaxy":  complete.PredictNothing,
	}
}
