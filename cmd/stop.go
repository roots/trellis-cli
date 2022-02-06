package cmd

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/template"

	"github.com/mitchellh/cli"
	"github.com/mitchellh/go-homedir"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/github"
	"github.com/roots/trellis-cli/lima"
	"github.com/roots/trellis-cli/trellis"
)

type StopCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewStopCommand(ui cli.Ui, trellis *trellis.Trellis) *StopCommand {
	c := &StopCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *StopCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *StopCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
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

	siteName, err := c.Trellis.FindSiteNameFromEnvironment("development", "")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	limaInstanceName := lima.ConvertToInstanceName(siteName)

	err = command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(c.UI),
	).Cmd("limactl", []string{"stop", limaInstanceName}).Run()

	if err != nil {
		return 1
	}

	err = command.WithOptions(
		command.WithTermOutput(),
	).Cmd("mutagen", []string{"sync", "terminate", limaInstanceName}).Run()

	if err != nil {
		return 1
	}

	return 0
}

func (c *StopCommand) Synopsis() string {
	return "Stops a VM and provisions the server with Trellis"
}

func (c *StopCommand) Help() string {
	helpText := `
Usage: trellis stop [options]

Stops a VM

Options:
  -h, --help show this help
`

	return strings.TrimSpace(helpText)
}
