package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

type ExecCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *ExecCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	var command string
	var cmdArgs []string

	switch len(args) {
	case 0:
		c.UI.Error("Error: missing COMMAND argument\n")
		c.UI.Output(c.Help())
		return 1
	default:
		command = args[0]
		cmdArgs = args
	}

	cmdPath, err := exec.LookPath(command)
	if err != nil {
		c.UI.Error(fmt.Sprintf("Error: %s not found", command))
		return 1
	}

	env := os.Environ()
	execErr := syscall.Exec(cmdPath, cmdArgs, env)
	if execErr != nil {
		c.UI.Error(fmt.Sprintf("Error running %s: %s", args[0], execErr))
		return 1
	}

	return 0
}

func (c *ExecCommand) Synopsis() string {
	return "Exec runs a command in the Trellis virtualenv"
}

func (c *ExecCommand) Help() string {
	helpText := `
Usage: trellis exec [options] COMMAND

Exec activates the Trellis virtual environment and executes the command specified.

Run a custom ansible-playbook command:

  $ trellis exec ansible-playbook --version

Arguments:
  COMMAND  Command to execute

Options:
  -h, --help show this help
`

	return CreateHelp("exec", c.Synopsis(), strings.TrimSpace(helpText))
}
