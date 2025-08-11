package cmd

import (
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/cli"
)

//go:embed files/hookbook.sh
var HookbookScript string

//go:embed files/trellis_cli_zsh_hook.sh
var ZshScript string

//go:embed files/trellis_cli_bash_hook.sh
var BashScript string

type ShellInitCommand struct {
	UI cli.Ui
}

func (c *ShellInitCommand) Run(args []string) int {
	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	var script string

	switch shell := args[0]; shell {
	case "zsh":
		script = ZshScript
	case "bash":
		script = BashScript
	default:
		c.UI.Error(fmt.Sprintf("Error: invalid shell name '%s'. Supported shells: bash, zsh", shell))
		c.UI.Output(c.Help())
		return 1
	}

	executable, _ := os.Executable()
	script = strings.Replace(script, "@SELF@", executable, -1)
	script = strings.Replace(script, "@HOOKBOOK@", HookbookScript, -1)
	c.UI.Output(script)

	return 0
}

func (c *ShellInitCommand) Synopsis() string {
	return "Prints a script which can be eval'd to set up Trellis' virtualenv integration in various shells."
}

func (c *ShellInitCommand) Help() string {
	helpText := `
Usage: trellis shell-init [options] SHELL

Prints a script which can be eval'd to set up Trellis' virtualenv integration in various shells.

To activate the integration, add one of the following lines to your shell profile (.zshrc, .bash_profile):

  eval "$(trellis shell-init bash)" # for bash
  eval "$(trellis shell-init zsh)"  # for zsh

Options:
  -h, --help  show this help
`
	return CreateHelp("shell-init", c.Synopsis(), strings.TrimSpace(helpText))
}
