package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/config"
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

	instance, ok := lima.GetInstance(siteName)

	if ok {
		if instance.Stopped() {
			c.UI.Info(fmt.Sprintf("%s Lima VM already stopped", color.GreenString("[✓]")))
			return 0
		} else {
			if err := instance.Stop(); err != nil {
				c.UI.Error("Error stopping lima instance")
				c.UI.Error(err.Error())
				return 1
			}
		}
	} else {
		c.UI.Info("Lima instance does not exist for this project. Start it first?")
	}

	err = command.WithOptions(
		command.WithTermOutput(),
	).Cmd("mutagen", []string{"sync", "terminate", instance.Name}).Run()

	if err != nil {
		return 1
	}

	dataDirs, err := config.Scope.DataDirs()
	if err != nil {
		c.UI.Error("could not determine XDG data dir. This is a trellis-cli bug.")
		return 1
	}

	err = deleteProxyRecords(dataDirs[0], c.Trellis.Environments["development"].AllHosts())
	if err != nil {
		c.UI.Error("Error deleting HTTP proxy record. This is a trellis-cli bug.")
		return 1
	}

	c.UI.Info(fmt.Sprintf("\n%s Lima VM stopped\n", color.GreenString("[✓]")))
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

func deleteProxyRecords(dataDir string, hosts []string) (err error) {
	for _, host := range hosts {
		path := filepath.Join(dataDir, host)
		err = os.Remove(path)

		if err != nil {
			return err
		}
	}

	return nil
}
