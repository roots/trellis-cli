package cmd

import (
	"flag"
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/pkg/trust"
	"github.com/roots/trellis-cli/trellis"
)

type VmTrustCommand struct {
	UI          cli.Ui
	Trellis     *trellis.Trellis
	flags       *flag.FlagSet
	noExportKey bool
	trustSystem bool
	site        string
}

func NewVmTrustCommand(ui cli.Ui, trellis *trellis.Trellis) *VmTrustCommand {
	c := &VmTrustCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmTrustCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.noExportKey, "no-export-key", false, "Skip exporting the private key to the host.")
	c.flags.BoolVar(&c.trustSystem, "trust-system", false, "Linux only: also write to /usr/local/share/ca-certificates and run sudo update-ca-certificates.")
	c.flags.StringVar(&c.site, "site", "", "Trust only the named site instead of every self-signed site.")
}

func (c *VmTrustCommand) Run(args []string) int {
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

	if c.trustSystem && runtime.GOOS != "linux" {
		c.UI.Error("Error: --trust-system is only supported on Linux.")
		return 1
	}

	manager, err := newVmManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	sites := c.selectSites()
	if len(sites) == 0 {
		if c.site != "" {
			c.UI.Error(fmt.Sprintf("Error: site %q not found, or it is not configured with ssl.enabled and ssl.provider: self-signed.", c.site))
			return 1
		}
		c.UI.Info("No sites in the development environment have ssl.enabled with provider: self-signed. Nothing to trust.")
		return 0
	}

	store, err := trust.Default(trust.Options{TrustSystem: c.trustSystem})
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	state, err := trust.Load()
	if err != nil {
		c.UI.Error("Error reading trust state: " + err.Error())
		return 1
	}

	instanceName, err := c.Trellis.GetVmInstanceName()
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	exitCode := 0
	firefoxHintShown := false

	for _, name := range sortedSiteNames(sites) {
		certPEM, err := manager.ReadRootFile("/etc/nginx/ssl/" + name + ".cert")
		if err != nil {
			c.UI.Error(fmt.Sprintf("%s: failed to read cert from VM: %s", name, err))
			c.UI.Error(fmt.Sprintf("       (does the cert exist at /etc/nginx/ssl/%s.cert? run `trellis provision development` if SSL was just enabled)", name))
			exitCode = 1
			continue
		}

		var keyPEM []byte
		if !c.noExportKey {
			keyData, keyErr := manager.ReadRootFile("/etc/nginx/ssl/" + name + ".key")
			if keyErr != nil {
				c.UI.Warn(fmt.Sprintf("%s: failed to export private key from VM: %s", name, keyErr))
			} else if len(keyData) == 0 {
				c.UI.Warn(fmt.Sprintf("%s: VM returned an empty private key; skipping key export.", name))
			} else {
				keyPEM = keyData
			}
		}

		out := trust.ApplySite(store, state, trust.SiteInput{
			Project:      c.Trellis.Path,
			Site:         name,
			InstanceName: instanceName,
			BaseDir:      app_paths.DataDir(),
			CertPEM:      certPEM,
			KeyPEM:       keyPEM,
		})

		if out.Err != nil {
			c.UI.Error(fmt.Sprintf("%s: %s", name, out.Err))
			if out.ErrHint != "" {
				c.UI.Error("       (" + out.ErrHint + ")")
			}
			exitCode = 1
			continue
		}

		c.UI.Info(fmt.Sprintf("%s: %s", name, out.Verb))
		for _, loc := range out.Locations {
			c.UI.Info("  - " + trust.FormatLocation(loc))
		}
		if out.NSS.FirefoxFound && out.NSS.CertutilMissing {
			firefoxHintShown = true
		}
	}

	if err := state.Save(); err != nil {
		c.UI.Error("Error saving trust state: " + err.Error())
		exitCode = 1
	}

	c.printSummary(trust.ExportDir(app_paths.DataDir(), instanceName, c.Trellis.Path), firefoxHintShown)

	return exitCode
}

func (c *VmTrustCommand) selectSites() map[string]*trellis.Site {
	out := map[string]*trellis.Site{}
	env := c.Trellis.Environments["development"]
	if env == nil {
		return out
	}
	for name, site := range env.WordPressSites {
		if c.site != "" && name != c.site {
			continue
		}
		if !site.SslEnabled() || site.SslProvider() != "self-signed" {
			continue
		}
		out[name] = site
	}
	return out
}

func (c *VmTrustCommand) printSummary(exportDir string, firefoxHint bool) {
	c.UI.Info("")
	c.UI.Info(fmt.Sprintf("Exported certs and keys live under %s", exportDir))
	if runtime.GOOS == "linux" {
		c.UI.Info(fmt.Sprintf("Set NODE_EXTRA_CA_CERTS, SSL_CERT_FILE, REQUESTS_CA_BUNDLE to %s for tools that read a single bundle.", filepath.Join(app_paths.DataDir(), "ca-bundle.pem")))
		c.UI.Info("Tools with statically-linked roots (some Go binaries, Java) ignore env-var trust roots; use --trust-system if you need system-wide trust.")
	}
	if firefoxHint {
		c.UI.Warn("")
		c.UI.Warn("Firefox is installed but `certutil` is not on PATH, so Firefox was not auto-trusted.")
		switch runtime.GOOS {
		case "darwin":
			c.UI.Warn("  Install with: brew install nss")
		case "linux":
			c.UI.Warn("  Install with: sudo apt install libnss3-tools  (or your distro's equivalent)")
		}
		c.UI.Warn("Then re-run `trellis vm trust`.")
		c.UI.Warn("Alternative: in Firefox set `security.enterprise_roots.enabled = true` in about:config to use the system trust store.")
	}
}

func sortedSiteNames(sites map[string]*trellis.Site) []string {
	names := make([]string, 0, len(sites))
	for name := range sites {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (c *VmTrustCommand) Synopsis() string {
	return "Trusts the VM's self-signed SSL certificates on the host."
}

func (c *VmTrustCommand) Help() string {
	helpText := `
Usage: trellis vm trust [options]

Extracts each self-signed SSL cert generated by Trellis for the development
environment, exports the cert and key to ~/.local/share/trellis/ssl/<project>/,
and trusts the cert in the host's platform trust stores.

On macOS the cert is added to the user's login keychain. On Linux the cert is
written to a per-user CA dir and a combined bundle.

When the certutil binary is available (from nss / libnss3-tools), the cert is
also added to every Firefox profile's NSS database.

This command requires the VM to be running.

Options:
      --site         Trust only this site instead of every self-signed site.
      --no-export-key  Skip writing the private key to the host.
      --trust-system Linux only: also install the cert system-wide
                     (writes to /usr/local/share/ca-certificates and runs
                     sudo update-ca-certificates).
  -h, --help         Show this help.
`
	return strings.TrimSpace(helpText)
}
