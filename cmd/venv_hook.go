package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
	"gopkg.in/alessio/shellescape.v1"
)

type VenvHookCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *VenvHookCommand) Run(args []string) int {
	_, ok := os.LookupEnv(trellis.OldPathEnvName)

	if c.Trellis.ActivateProject() {
		if !ok {
			fmt.Fprintf(color.Error, "[trellis] \x1b[1;32mactivated env\x1b[0m\n")
			c.UI.Output(fmt.Sprintf("export %s=%s", trellis.VenvEnvName, shellescape.Quote(c.Trellis.Virtualenv.Path)))
			c.UI.Output(fmt.Sprintf("export %s=%s", trellis.OldPathEnvName, shellescape.Quote(c.Trellis.Virtualenv.OldPath)))
			c.UI.Output(fmt.Sprintf("export %s=%s:%s", trellis.PathEnvName, c.Trellis.Virtualenv.BinPath, shellescape.Quote(c.Trellis.Virtualenv.OldPath)))
		}
	} else {
		if ok {
			fmt.Fprintf(color.Error, "[trellis] \x1b[1;31mdeactivated env\x1b[0m\n")
			path := os.Getenv(trellis.OldPathEnvName)
			c.UI.Output(fmt.Sprintf("unset %s", trellis.VenvEnvName))
			c.UI.Output(fmt.Sprintf("unset %s", trellis.OldPathEnvName))
			c.UI.Output(fmt.Sprintf("export %s=%s", trellis.PathEnvName, shellescape.Quote(path)))
		}
	}

	return 0
}

func (c *VenvHookCommand) Synopsis() string {
	return "Virtualenv shell hook."
}

func (c *VenvHookCommand) Help() string {
	helpText := `
Usage: trellis venv hook

Virtualenv shell hook. This shouldn't be manually run.

See shell-init for installation instructions.

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
