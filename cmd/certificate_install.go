package cmd

import (
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/certificates"
	"github.com/roots/trellis-cli/trellis"
	"go.step.sm/crypto/pemutil"
)

type CertificateInstallCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	path    string
}

func NewCertificateInstallCommand(ui cli.Ui, trellis *trellis.Trellis) *CertificateInstallCommand {
	c := &CertificateInstallCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *CertificateInstallCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.path, "path", "", "Local path to custom root certificate to install")
}

func (c *CertificateInstallCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 1}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	environment := "development"

	if len(args) == 1 {
		environment = args[0]
		environmentErr := c.Trellis.ValidateEnvironment(environment)
		if environmentErr != nil {
			c.UI.Error(environmentErr.Error())
			return 1
		}
	}

	cert, err := c.fetchRootCertificate(environment)
	if err != nil {
		c.UI.Error("Error fetching root certificate from server:")
		c.UI.Error(err.Error())
		c.UI.Error("The server (likely Vagrant virtual machine) must be running and have been provisioned with an SSL enabled site.")
		return 1
	}

	if certificates.Trusted(cert) {
		c.UI.Info("Root certificate already installed and trusted")
		return 0
	}

	if err := certificates.InstallFile(c.path); err != nil {
		c.UI.Error("Error installing root certificate to truststore:")
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Info(fmt.Sprintf("Certificate %s has been installed.\n", c.path))
	if text, err := certificates.ShortText(cert); err == nil {
		c.UI.Info(text)
	}

	c.UI.Info("Note: your web browser(s) will need to be restarted for this to take effect.")

	return 0
}

func (c *CertificateInstallCommand) Synopsis() string {
	return "Installs a root certificate in the system truststore"
}

func (c *CertificateInstallCommand) Help() string {
	helpText := `
Usage: trellis certificate install [options] [ENVIRONMENT]

Installs a root certificate in the system truststore. This allows your local
computer to trust the "self-signed" root CA (certificate authority) that Trellis
uses in development which avoids insecure warnings in your web browsers.

By default this integrates with a Trellis server/VM and requires that it's running.
However, the --path option can be used to specify any root certificate making this
command useful for non-Trellis use cases too.

Note: browsers may have to be restarted after running this command for it to take effect.

Install a non-default root certificate via a local path:

  $ trellis certificate install --path ~/certs/root.crt

Arguments:
  ENVIRONMENT Name of environment (default: development)

Options:
  -h, --help  show this help
  --path      local path to custom root certificate to install
`

	return strings.TrimSpace(helpText)
}

func (c *CertificateInstallCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment(c.flags)
}

func (c *CertificateInstallCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--path": complete.PredictNothing,
	}
}

func (c *CertificateInstallCommand) fetchRootCertificate(environment string) (cert *x509.Certificate, err error) {
	if c.path == "" {
		c.path = certificates.RootCertificatePath(c.Trellis.ConfigPath())
	}
	siteName, _ := c.Trellis.FindSiteNameFromEnvironment(environment, "")
	host := c.Trellis.SiteFromEnvironmentAndName(environment, siteName).MainHost()

	certBytes, err := certificates.FetchRootCertificate(c.path, host)

	if err = os.MkdirAll(filepath.Dir(c.path), os.ModePerm); err != nil {
		return nil, err
	}

	if err = os.WriteFile(c.path, certBytes, os.ModePerm); err != nil {
		return nil, err
	}

	cert, err = pemutil.ParseCertificate(certBytes)

	if err != nil {
		return nil, err
	}

	return cert, nil
}
