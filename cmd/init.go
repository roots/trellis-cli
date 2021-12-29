package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func NewInitCommand(ui cli.Ui, trellis *trellis.Trellis) *InitCommand {
	c := &InitCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

type InitCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	force   bool
}

func (c *InitCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.force, "force", false, "Force initialization by re-creating the virtualenv")
}

func (c *InitCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		c.UI.Error(err.Error())
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

	if err := c.Trellis.CreateConfigDir(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info("Initializing project\n")

	if ok, _ := c.Trellis.Virtualenv.Installed(); !ok {
		c.UI.Info("virtualenv not found")
		spinner := NewSpinner(
			SpinnerCfg{
				Message:     "Installing virtualenv",
				FailMessage: "Error installing virtualenv",
			},
		)
		spinner.Start()
		c.Trellis.Virtualenv.Install()
		spinner.Stop()
	}

	if c.force {
		spinner := NewSpinner(
			SpinnerCfg{
				Message:     "Deleting existing virtualenv",
				FailMessage: "Error deleting virtualenv",
			},
		)
		spinner.Start()
		err := os.RemoveAll(c.Trellis.Virtualenv.Path)

		if err != nil {
			spinner.StopFail()
			c.UI.Error(err.Error())
			return 1
		}

		spinner.Stop()
	}

	if !c.Trellis.Virtualenv.Initialized() {
		spinner := NewSpinner(
			SpinnerCfg{
				Message:     "Creating virtualenv",
				FailMessage: "Error creating virtualenv",
				StopMessage: fmt.Sprintf("Created virtualenv (%s)", c.Trellis.Virtualenv.Path),
			},
		)

		spinner.Start()
		err := c.Trellis.Virtualenv.Create()
		if err != nil {
			spinner.StopFail()
			c.UI.Error(err.Error())
			return 1
		}

		c.Trellis.VenvInitialized = true
		spinner.Stop()
	}

	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Installing dependencies (this can take a minute...)",
			FailMessage: "Error installing dependencies",
			StopMessage: "Installing dependencies",
		},
	)
	spinner.Start()
	pipCmd := exec.Command("pip", "install", "-r", "requirements.txt")
	errorOutput := &bytes.Buffer{}
	pipCmd.Stderr = errorOutput
	err := pipCmd.Run()

	if err != nil {
		spinner.StopFail()
		c.UI.Error(errorOutput.String())
		return 1
	}

	spinner.Stop()
	return 0
}

func (c *InitCommand) Synopsis() string {
	return "Initializes an existing Trellis project"
}

func (c *InitCommand) Help() string {
	helpText := `
Usage: trellis init [options]

Initializes an existing Trellis project to be managed by trellis-cli.
The initialization process does two things:

1. installs virtualenv if necessary (see below for details)
2. creates a virtual environment specific to the project to manage dependencies
3. installs dependencies via pip (specified by requirements.txt in your Trellis project)

trellis-cli will attempt to use an already installed method to manage virtualenvs
and only fallback to installing virtualenv if necessary:

1. if python3 is installed, use built-in virtualenv feature
2. use virtualenv command if available
3. finally install virtualenv at $HOME/.trellis/virtualenv

To learn more about virtual environments, see https://docs.python.org/3/tutorial/venv.html

This command is idempotent meaning it can be run multiple times without side-effects.

  $ trellis init

Force initialization by re-creating the existing virtualenv

  $ trellis init --force

Options:
      --force  Force init by re-creating the virtualenv
  -h, --help   show this help
`

	return strings.TrimSpace(helpText)
}
