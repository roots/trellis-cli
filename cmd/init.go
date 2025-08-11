package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type WrappedIOWriter struct {
	writer io.Writer
}

func (w *WrappedIOWriter) Write(p []byte) (n int, err error) {
	return w.writer.Write(p)
}

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

	c.UI.Info("Initializing project...\n")
	ok, virtualenvCmd := c.Trellis.Virtualenv.Installed()

	if !ok {
		c.UI.Error("virtualenv not found. trellis-cli looked for two options:")
		c.UI.Error("  1. python3's built-in `venv` module")
		c.UI.Error("  2. standalone `virtualenv` command")
		c.UI.Error("")
		virtualenvError(c.UI)
		return 1
	}

	if c.force {
		if err := c.deleteVirtualenv(); err != nil {
			return 1
		}
	}

	if !c.Trellis.Virtualenv.Initialized() {
		if err := c.createVirtualenv(virtualenvCmd); err != nil {
			return 1
		}
	}

	if err := c.upgradePip(); err != nil {
		return 1
	}

	if err := c.pipInstall(); err != nil {
		return 1
	}

	if err := c.Trellis.Virtualenv.UpdateBinShebangs("ansible*"); err != nil {
		c.UI.Error("Error while initializing project in a directory path that contains spaces.")
		c.UI.Error("Python's virtualenv does not properly handle paths with spaces in them. trellis-cli attempted to automatically fix the bin scripts as a workaround but encountered an error:")
		c.UI.Error(err.Error())
		c.UI.Error("As an alternative, you can re-create this project in a path without spaces.")
		c.UI.Error("Or you can open an issue to let us know: https://github.com/roots/trellis-cli")
	}

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

	return CreateHelp("init", c.Synopsis(), strings.TrimSpace(helpText))
}

func virtualenvError(ui cli.Ui) {
	ui.Error("Without virtualenv, a Python virtual environment cannot be created and the required dependencies (eg: Ansible) can't be installed either.")
	ui.Error("")
	ui.Error("There are two options:")
	ui.Error("  1. Ensure Python 3 is installed and the `python3` command works. trellis-cli will use python3's built-in venv feature.")
	ui.Error("     Ubuntu/Debian users (including Windows WSL): venv is not built-in, to install it run `sudo apt-get install python3-pip python3-venv`")
	ui.Error("")
	ui.Error("  2. Disable trellis-cli's virtualenv feature, and manage dependencies manually, by changing the 'virtualenv_integration' configuration setting to 'false'.")
}

func (c *InitCommand) deleteVirtualenv() error {
	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Deleting existing virtualenv",
			FailMessage: "Error deleting virtualenv",
		},
	)
	_ = spinner.Start()
	err := c.Trellis.Virtualenv.Delete()

	if err != nil {
		_ = spinner.StopFail()
		c.UI.Error(err.Error())
		return err
	}

	_ = spinner.Stop()

	return nil
}

func (c *InitCommand) createVirtualenv(virtualenvCmd *exec.Cmd) error {
	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Creating virtualenv",
			FailMessage: "Error creating virtualenv",
			StopMessage: fmt.Sprintf("Created virtualenv (%s)", c.Trellis.Virtualenv.Path),
		},
	)

	_ = spinner.Start()
	err := c.Trellis.Virtualenv.Create()
	if err != nil {
		_ = spinner.StopFail()
		c.UI.Error(err.Error())
		c.UI.Error("")
		c.UI.Error("Project initialization failed due to the error above.")
		c.UI.Error("")
		c.UI.Error("trellis-cli tried to create a virtual environment but failed.")
		c.UI.Error(fmt.Sprintf("  => %s", virtualenvCmd.String()))
		c.UI.Error("")
		virtualenvError(c.UI)
		return err
	}

	c.Trellis.VenvInitialized = true
	_ = spinner.Stop()

	return nil
}

func (c *InitCommand) upgradePip() error {
	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Ensure pip is up to date",
			FailMessage: "Error upgrading pip",
		},
	)
	_ = spinner.Start()
	pipUpgradeOutput, err := command.Cmd("python3", []string{"-m", "pip", "install", "--upgrade", "pip"}).CombinedOutput()

	if err != nil {
		_ = spinner.StopFail()
		c.UI.Error(string(pipUpgradeOutput))
		return err
	}

	_ = spinner.Stop()
	return nil
}

func (c *InitCommand) pipInstall() error {
	spinner := NewSpinner(
		SpinnerCfg{
			Message:     "Installing dependencies (this can take a minute...)",
			FailMessage: "Error installing dependencies",
			StopMessage: "Dependencies installed",
		},
	)
	_ = spinner.Start()
	pipCmd := command.Cmd("pip", []string{"install", "-r", "requirements.txt"})

	// Wrap pipCmd's Stdout in a custom writer that only displays output once the timer has elapsed.
	timer := time.NewTimer(30 * time.Second)
	writer := &WrappedIOWriter{writer: io.Discard}
	pipCmd.Stdout = writer

	go func() {
		<-timer.C
		_ = spinner.Pause()
		writer.writer = os.Stdout
		c.UI.Warn("\n\npip install taking longer than expected. Switching to verbose output:\n")
	}()

	errorOutput := &bytes.Buffer{}
	pipCmd.Stderr = errorOutput
	err := pipCmd.Run()

	if err != nil {
		_ = spinner.StopFail()
		c.UI.Error(errorOutput.String())
		return err
	}

	_ = spinner.Stop()
	return nil
}
