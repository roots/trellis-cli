package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/trust"
	"github.com/roots/trellis-cli/trellis"
)

type VmUntrustCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	site    string
}

func NewVmUntrustCommand(ui cli.Ui, trellis *trellis.Trellis) *VmUntrustCommand {
	c := &VmUntrustCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmUntrustCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.site, "site", "", "Untrust only the named site instead of every entry for the project.")
}

func (c *VmUntrustCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	if err := (&CommandArgumentValidator{required: 0, optional: 0}).validate(c.flags.Args()); err != nil {
		c.UI.Error(err.Error())
		c.UI.Output(c.Help())
		return 1
	}

	state, err := trust.Load()
	if err != nil {
		c.UI.Error("Error reading trust state: " + err.Error())
		return 1
	}

	// Untrust is location-driven: it iterates the locations recorded at
	// trust time and removes from each, so no Options toggles are needed.
	// sudo will be requested if a Linux system-CA entry is present.
	store, err := trust.Default(trust.Options{})
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	project := c.Trellis.Path
	entries := state.EntriesForProject(project)
	if c.site != "" {
		filtered := entries[:0]
		for _, e := range entries {
			if e.Site == c.site {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	if len(entries) == 0 {
		c.UI.Info(fmt.Sprintf("nothing to untrust for %s", project))
		return 0
	}

	exitCode := 0
	for _, entry := range entries {
		out := trust.RevokeSite(store, state, project, entry)
		if out.Err != nil {
			c.UI.Error(fmt.Sprintf("%s: failed to untrust: %s", out.Site, out.Err))
			if out.ErrHint != "" {
				c.UI.Error("       (" + out.ErrHint + ")")
			}
			exitCode = 1
			continue
		}

		if len(out.Cleaned) == 0 {
			c.UI.Info(fmt.Sprintf("%s: removed (no host changes needed)", out.Site))
			continue
		}
		c.UI.Info(fmt.Sprintf("%s: untrusted", out.Site))
		for _, loc := range out.Cleaned {
			c.UI.Info("  - " + trust.FormatLocation(loc))
		}
	}

	if err := state.Save(); err != nil {
		c.UI.Error("Error saving trust state: " + err.Error())
		exitCode = 1
	}

	return exitCode
}

func (c *VmUntrustCommand) Synopsis() string {
	return "Removes trusted SSL certificates that vm trust added for this project."
}

func (c *VmUntrustCommand) Help() string {
	helpText := `
Usage: trellis vm untrust [options]

Removes trust entries that 'trellis vm trust' previously added for this
project, and deletes the exported cert and key files. Entries that were not
created by trellis-cli are left alone.

Untrust always reverses whatever was recorded at trust time — including
Linux system-wide CA entries from --trust-system — so the cleanup is
symmetric with the original trust call. sudo will be requested if a
system-CA entry is present.

Options:
      --site         Untrust only the named site.
  -h, --help         Show this help.
`
	return strings.TrimSpace(helpText)
}
