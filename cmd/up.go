package cmd

import (
	"flag"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type UpCommand struct {
	UI          cli.Ui
	Trellis     *trellis.Trellis
	flags       *flag.FlagSet
	noGalaxy    bool
	noProvision bool
	debug       bool
}

func NewUpCommand(ui cli.Ui, trellis *trellis.Trellis) *UpCommand {
	c := &UpCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *UpCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.noGalaxy, "no-galaxy", false, "Skip Ansible Galaxy install")
	c.flags.BoolVar(&c.noProvision, "no-provision", false, "Skip provisioning")
	c.flags.BoolVar(&c.debug, "debug", false, "Enable vagrant's debug mode")
}

func (c *UpCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

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

	if !c.noGalaxy {
		galaxyInstallCommand := &GalaxyInstallCommand{c.UI, c.Trellis}
		galaxyInstallCommand.Run([]string{})
	}

	vagrantArgs := []string{"up"}

	if c.noProvision {
		vagrantArgs = append(vagrantArgs, "--no-provision")
	}

	if c.debug {
		vagrantArgs = append(vagrantArgs, "--debug")
	}

	vagrantUp := command.WithOptions(command.WithTermOutput(), command.WithLogging(c.UI)).Cmd("vagrant", vagrantArgs)

	env := os.Environ()
	// To allow moc.CmdCommand injects its environment variables.
	if vagrantUp.Env != nil {
		env = vagrantUp.Env
	}

	vagrantUp.Env = append(env, "SKIP_GALAXY=true")

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

Start VM and skip Galaxy install:

  $ trellis up --no-galaxy

Start VM and display debug output:

  $ trellis up --debug

Options:
      --no-provision (default: false) Skip provisioning
      --no-galaxy    (default: false) Skip Ansible Galaxy install
      --debug        (default: false) Enable vagrant's debug mode
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
		"--no-galaxy":    complete.PredictNothing,
		"--debug":        complete.PredictNothing,
	}
}
