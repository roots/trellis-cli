package cmd

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/lima"
	"github.com/roots/trellis-cli/trellis"
)

type VmSudoersCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *VmSudoersCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	hostResolver := lima.NewHostsFileResolver([]string{})
	cmd := hostResolver.SudoersCommand()

	c.UI.Info(fmt.Sprintf("%%staff ALL=(root:wheel) NOPASSWD:NOSETENV: %s", strings.Join(cmd, " ")))

	return 0
}

func (c *VmSudoersCommand) Synopsis() string {
	return "Deletes the development virtual machine."
}

func (c *VmSudoersCommand) Help() string {
	helpText := `
Usage: trellis vm sudoers [options]

Generates the content of the /etc/sudoers.d/trellis file.
The content is written to stdout, NOT to the file. This command must not run as the root.

This allows trellis-cli to update your /etc/hosts file without having to enter your sudo password.

$ trellis sudoers | sudo tee /etc/sudoers.d/trellis

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}
