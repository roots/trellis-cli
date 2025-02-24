package cmd

import (
	"flag"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

type VmShellCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	workdir string
}

func NewVmShellCommand(ui cli.Ui, trellis *trellis.Trellis) *VmShellCommand {
	c := &VmShellCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmShellCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.workdir, "workdir", "", "Working directory to start the shell in.")
}

func (c *VmShellCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	instanceName, err := c.Trellis.GetVmInstanceName()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	siteName, _, err := c.Trellis.MainSiteFromEnvironment("development")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	manager, err := newVmManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	if c.workdir == "" {
		c.workdir = "/srv/www/" + siteName + "/current"
	}

	shellArgs := []string{"--workdir", c.workdir}
	shellArgs = append(shellArgs, args...)

	if err := manager.OpenShell(instanceName, c.workdir, args); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *VmShellCommand) Synopsis() string {
	return "Executes shell in the VM"
}

func (c *VmShellCommand) Help() string {
	helpText := `
Usage: trellis vm shell [options] [-- COMMAND]

Executes shell in the development virtual machine.

Any arguments after the -- separator will be passed to the shell:

  $ trellis vm shell -- whoami

Start shell in a specific working directory:

  $ trellis vm shell --workdir /srv/www/example.com/shared

Run a command in a specific working directory:

  $ trellis vm shell --workdir /srv/www/example.com/shared -- ls

Arguments:
  COMMAND  Command to execute

Options:
      --workdir  Working directory to start the shell in.
  -h, --help     Show this help
`

	return strings.TrimSpace(helpText)
}
