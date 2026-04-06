package cmd

import (
	"flag"
	"runtime"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/wsl"
	"github.com/roots/trellis-cli/trellis"
)

type VmSyncCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewVmSyncCommand(ui cli.Ui, trellis *trellis.Trellis) *VmSyncCommand {
	c := &VmSyncCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmSyncCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *VmSyncCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	if err := commandArgumentValidator.validate(args); err != nil {
		c.UI.Error(err.Error())
		c.UI.Output(c.Help())
		return 1
	}

	if runtime.GOOS != "windows" {
		c.UI.Error("'trellis vm sync' is only supported on Windows (WSL2).")
		return 1
	}

	instanceName, err := c.Trellis.GetVmInstanceName()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	manager, err := newVmManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	wslManager, ok := manager.(*wsl.Manager)
	if !ok {
		c.UI.Error("'trellis vm sync' requires the WSL backend.")
		return 1
	}

	if err := wslManager.SyncBack(instanceName); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *VmSyncCommand) Synopsis() string {
	return "Syncs project files from the WSL2 VM back to Windows"
}

func (c *VmSyncCommand) Help() string {
	helpText := `
Usage: trellis vm sync [options]

Syncs project files from the WSL2 VM back to the Windows filesystem.

On Windows, your project files live on the WSL2 ext4 filesystem for
performance. This command copies changes back to the Windows side so
that GitHub Desktop and other Windows tools can see your latest work.

This sync runs automatically during 'trellis vm stop'. Use this command
to sync manually without stopping the VM (e.g. before pushing from
GitHub Desktop).

Direction: WSL → Windows (one-way). Generated files like vendor/ and
node_modules/ are excluded.

Options:
  -h, --help  Show this help
`

	return strings.TrimSpace(helpText)
}
