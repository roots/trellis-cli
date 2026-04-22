package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/mattn/go-isatty"
	"github.com/roots/trellis-cli/pkg/vm"
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

	hostResolver := vm.NewHostsFileResolver([]string{})
	cmd := hostResolver.SudoersCommand()
	line := fmt.Sprintf("%%staff ALL=(root:wheel) NOPASSWD:NOSETENV: %s", strings.Join(cmd, " "))

	if stdoutIsTerminal() {
		c.UI.Warn("The following sudoers rule lets trellis-cli update /etc/hosts without prompting for your password.")
		c.UI.Warn("")
		c.UI.Warn("To install it, re-run this command and pipe the output to tee:")
		c.UI.Warn("")
		c.UI.Warn("  trellis vm sudoers | sudo tee /etc/sudoers.d/trellis")
		c.UI.Warn("")
		c.UI.Warn("Generated rule:")
		c.UI.Warn("")
	}

	c.UI.Info(line)

	return 0
}

func stdoutIsTerminal() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

func (c *VmSudoersCommand) Synopsis() string {
	return "Generates sudoers content for passwordless updating of /etc/hosts"
}

func (c *VmSudoersCommand) Help() string {
	helpText := `
Usage: trellis vm sudoers [options]

Generates the content of the /etc/sudoers.d/trellis file.
This allows trellis-cli to update your /etc/hosts file without having to enter your sudo password.

The content is written to stdout, NOT to the file. This command must not run as the root as shown below.

$ trellis vm sudoers | sudo tee /etc/sudoers.d/trellis

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}
