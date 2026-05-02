package cmd

import (
	"flag"
	"fmt"
	"os"
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

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	if err := commandArgumentValidator.validate(c.flags.Args()); err != nil {
		c.UI.Error(err.Error())
		c.UI.Output(c.Help())
		return 1
	}

	releaseLock, err := trust.AcquireLock(&cli.UiWriter{Ui: c.UI})
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}
	defer releaseLock()

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
		input := trust.TrustInput{
			CertPath:        entry.CertPath,
			Fingerprint:     entry.Fingerprint,
			FingerprintSHA1: entry.FingerprintSHA1,
			Label:           entry.Label,
		}

		cleaned, err := store.Untrust(input, entry.Locations)
		if err != nil {
			c.UI.Error(fmt.Sprintf("%s: failed to untrust: %s", entry.Site, err))
			c.UI.Error(fmt.Sprintf("       (state preserved so you can re-run `trellis vm untrust --site %s`)", entry.Site))
			exitCode = 1
			continue
		}

		// Drop exported cert+key files only after store cleanup succeeded.
		if entry.CertPath != "" {
			_ = os.Remove(entry.CertPath)
		}
		if entry.KeyPath != "" {
			_ = os.Remove(entry.KeyPath)
		}

		state.Remove(project, entry.Site)

		if len(cleaned) == 0 {
			c.UI.Info(fmt.Sprintf("%s: removed (no host changes needed)", entry.Site))
		} else {
			c.UI.Info(fmt.Sprintf("%s: untrusted", entry.Site))
			for _, loc := range cleaned {
				c.UI.Info("  - " + trust.FormatLocation(loc))
			}
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
