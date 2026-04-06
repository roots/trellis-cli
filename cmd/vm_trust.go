package cmd

import (
	"flag"
	"runtime"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/wsl"
	"github.com/roots/trellis-cli/trellis"
)

type VmTrustCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
}

func NewVmTrustCommand(ui cli.Ui, trellis *trellis.Trellis) *VmTrustCommand {
	c := &VmTrustCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmTrustCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
}

func (c *VmTrustCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if windowsHostRequired(c.Trellis, c.UI, "vm trust") {
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
		c.UI.Error("'trellis vm trust' is only supported on Windows (WSL2).")
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
		c.UI.Error("'trellis vm trust' is only supported with the WSL backend.")
		return 1
	}

	distro := "trellis-" + strings.ReplaceAll(instanceName, ".", "-")

	if err := wslManager.TrustSslCerts(distro); err != nil {
		c.UI.Error("Error trusting SSL certificates: " + err.Error())
		return 1
	}

	return 0
}

func (c *VmTrustCommand) Synopsis() string {
	return "Imports SSL certificates from the VM into the Windows trust store"
}

func (c *VmTrustCommand) Help() string {
	helpText := `
Usage: trellis vm trust [options]

Extracts self-signed SSL certificates from the WSL2 distro and imports
them into the Windows Trusted Root Certification Authorities store.

This is automatically done during 'trellis vm start' on initial setup.
Run this command after re-provisioning with SSL enabled to trust the
new certificates without restarting the VM.

Requires admin privileges (a UAC prompt will appear).

Options:
  -h, --help  show this help
`
	return strings.TrimSpace(helpText)
}
