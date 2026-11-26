package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/cli"
)

type ConvergeUpgradeCommand struct {
	UI      cli.Ui
	Version string
	flags   *flag.FlagSet
	conn    connection
}

func NewConvergeUpgradeCommand(ui cli.Ui, version string) *ConvergeUpgradeCommand {
	c := &ConvergeUpgradeCommand{UI: ui, Version: version}
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.conn.register(c.flags)
	return c
}

func (c *ConvergeUpgradeCommand) Run(args []string) int {
	args, err := parseArgs(c.flags, args)
	if err != nil {
		return 1
	}
	if len(args) != 1 {
		c.UI.Error("Usage: trellis converge upgrade <environment>")
		return 1
	}
	env := args[0]
	p := openProject(c.UI)
	if p == nil {
		return 1
	}
	ctx, cancel := signalContext()
	defer cancel()

	compiled := compileEnv(c.UI, p, env, c.Version)
	if compiled == nil {
		return 1
	}
	out := &uiWriter{ui: c.UI}
	s, srv, err := c.conn.session(ctx, out, compiled)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	res, err := s.Upgrade(ctx, srv)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	if len(res.Upgraded) == 0 {
		c.UI.Info("  ✓ nothing to upgrade")
	} else {
		c.UI.Info(fmt.Sprintf("  ✓ upgraded %d packages", len(res.Upgraded)))
	}
	if len(res.Waiting) > 0 {
		c.UI.Info(fmt.Sprintf("  %d packages still have updates that auto_updates doesn't allow: %s", len(res.Waiting), strings.Join(res.Waiting, ", ")))
	}
	if res.RebootRequired {
		c.UI.Warn("  the server needs a reboot to finish installing updates")
	}
	return 0
}

func (c *ConvergeUpgradeCommand) Synopsis() string {
	return "Install waiting package updates on an environment's server now"
}

func (c *ConvergeUpgradeCommand) Help() string {
	return `Usage: trellis converge upgrade [options] <environment>

Runs unattended-upgrades on the server now instead of waiting for its
nightly run, in one transaction. It installs exactly what auto_updates
allows (with auto_updates: off, what "all" would), keeps Trellis's config
files and skips packages pinned in trellis.lock. Services restart as the
packages ask (PHP-FPM and MariaDB for a few seconds).

Run it after a server's first apply: fresh images are usually behind.

Options:
` + connectionHelp
}
