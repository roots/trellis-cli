package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
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

	if ok, _ := c.Trellis.Virtualenv.Installed(); !ok {
		c.UI.Info("virtualenv not found")
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Installing virtualenv..."
		s.FinalMSG = color.GreenString("\n✓ virtualenv installed")
		s.Start()
		c.Trellis.Virtualenv.Install()
		s.Stop()
	}

	if c.force {
		err := os.RemoveAll(c.Trellis.Virtualenv.Path)

		if err != nil {
			c.UI.Error(fmt.Sprintf("Error deleting virtualenv: %s", err))
			return 1
		}

		c.UI.Info(color.GreenString("✓ Existing virtualenv deleted"))
	}

	if !c.Trellis.Virtualenv.Initialized() {
		c.UI.Info(fmt.Sprintf("Creating virtualenv in %s", c.Trellis.Virtualenv.Path))

		err := c.Trellis.Virtualenv.Create()
		if err != nil {
			c.UI.Error(fmt.Sprintf("Error creating virtualenv: %s", err))
			return 1
		}

		c.Trellis.VenvInitialized = true
		c.UI.Info(color.GreenString("✓ virtualenv created"))
	}

	s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
	s.Suffix = " Installing dependencies => pip install -r requirements.txt (this can take a minute...)"
	s.FinalMSG = "\n"
	s.Start()
	pipCmd := exec.Command("pip", "install", "-r", "requirements.txt")
	errorOutput := &bytes.Buffer{}
	pipCmd.Stderr = errorOutput
	err := pipCmd.Run()
	s.Stop()

	if err != nil {
		c.UI.Error("✘ Error installing dependencies\n")
		c.UI.Error(errorOutput.String())
		return 1
	}

	c.UI.Info(color.GreenString("✓ Dependencies installed"))
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
