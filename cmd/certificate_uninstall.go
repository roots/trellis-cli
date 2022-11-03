package cmd

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/certificates"
	"github.com/roots/trellis-cli/trellis"
)

type CertificateUninstallCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	path    string
}

func NewCertificateUninstallCommand(ui cli.Ui, trellis *trellis.Trellis) *CertificateUninstallCommand {
	c := &CertificateUninstallCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *CertificateUninstallCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.path, "path", "", "Local path to custom root certificate to uninstall")
}

func (c *CertificateUninstallCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	if c.path == "" {
		c.path = certificates.RootCertificatePath(c.Trellis.ConfigPath())
	}

	if _, err := os.Stat(c.path); os.IsNotExist(err) {
		c.UI.Error(fmt.Sprintf("Root certificate not found: %s", c.path))
		return 1
	}

	if err := certificates.UninstallFile(c.path); err != nil {
		c.UI.Error("Error uninstalling root certificate to truststore:")
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("Certificate %s has been removed.\n", c.path))
	c.UI.Info("Note: your web browser(s) will need to be restarted for this to take effect.")

	return 0
}

func (c *CertificateUninstallCommand) Synopsis() string {
	return "Uninstalls a root certificate in the system truststore"
}

func (c *CertificateUninstallCommand) Help() string {
	helpText := `
Usage: trellis certificate uninstall [options]

Uninstalls a root certificate in the system truststore. This will stop your computer/browser
from trusting the root certificate authority.

Note: browsers may have to be restarted after running this command for it to take effect.

Uninstall a non-default root certificate via a local path:

  $ trellis certificate uninstall --path ~/certs/root.crt

Options:
  -h, --help  show this help
  --path      local path to custom root certificate to uninstall
`

	return strings.TrimSpace(helpText)
}

func (c *CertificateUninstallCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment(c.flags)
}

func (c *CertificateUninstallCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--path": complete.PredictNothing,
	}
}
