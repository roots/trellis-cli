package cmd

import (
	"flag"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
)

type UpCommand struct {
	UI          cli.Ui
	Trellis     *trellis.Trellis
	flags       *flag.FlagSet
	noGalaxy    bool
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
	c.flags.BoolVar(&c.noGalaxy, "no-galaxy", false, "Disable Ansible Galaxy auto-install")
	c.flags.BoolVar(&c.noProvision, "no-provision", false, "Disable provisioning")
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

	argCountErr := validateArgumentCount(args, 0, 0)
	if argCountErr != nil {
		c.UI.Error(argCountErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	vagrantArgs := []string{"up"}

	if c.noProvision {
		vagrantArgs = append(vagrantArgs, "--no-provision")
	}

	vagrantUp := execCommand("vagrant", vagrantArgs...)

	if c.noGalaxy {
		vagrantUp.Env = append(os.Environ(), "SKIP_GALAXY=true")
	}

	logCmd(vagrantUp, c.UI, true)
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

Start VM without running Ansible Galaxy:

  $ trellis up --no-galaxy

Options:
      --no-galaxy    (default: false) Disable Ansible Galaxy auto-install
      --no-provision (default: false) Disable provisioning
  -h, --help         show this help
`

	return strings.TrimSpace(helpText)
}

func (c *UpCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *UpCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--no-galaxy":    complete.PredictNothing,
		"--no-provision": complete.PredictNothing,
	}
}
