package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
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
			c.exportEnv(trellis.VenvEnvName, c.Trellis.Virtualenv.Path)
			c.exportEnv(trellis.OldPathEnvName, c.Trellis.Virtualenv.OldPath)
			newPath := fmt.Sprintf("%s:%s", c.Trellis.Virtualenv.BinPath, c.Trellis.Virtualenv.OldPath)
			c.exportEnv(trellis.PathEnvName, newPath)
		}
	} else {
		if ok {
			fmt.Fprintf(color.Error, "[trellis] \x1b[1;31mdeactivated env\x1b[0m\n")
			c.UI.Output(fmt.Sprintf("unset %s", trellis.VenvEnvName))
			c.UI.Output(fmt.Sprintf("unset %s", trellis.OldPathEnvName))
			c.exportEnv(trellis.PathEnvName, os.Getenv(trellis.OldPathEnvName))
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

func (c *VenvHookCommand) exportEnv(key string, value string) {
	c.UI.Output(fmt.Sprintf("export %s=%s", key, shellescape.Quote(value)))
}
