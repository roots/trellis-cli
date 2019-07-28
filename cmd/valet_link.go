package cmd

import (
	"fmt"
	"github.com/fatih/color"
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type ValetLinkCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *ValetLinkCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	var environment string

	switch len(args) {
	case 0:
		environment = "development"
	case 1:
		environment = args[0]
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 0 or 1, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	config, ok := c.Trellis.Environments[environment]
	if !ok {
		c.UI.Error(fmt.Sprintf("Error: %s is not a valid environment", environment))
		return 1
	}

	c.UI.Info(fmt.Sprintf("Linking environment %s...", environment))

	for key, site := range config.WordPressSites {
		c.UI.Info(fmt.Sprintf("Linking site %s...\n", key))

		canonical, _ := c.Trellis.HostsFromDomain(site.SiteHosts[0].Canonical, environment)
		app := strings.TrimSuffix(canonical.String(), "." + canonical.TLD)

		isSiteSslEnabled := site.Ssl["enabled"] == true

		valetArgs := []string{"link"}

		if isSiteSslEnabled {
			valetArgs = append(valetArgs, "--secure")
		}
		valetArgs = append(valetArgs, app)

		valetLink := execCommand("valet", valetArgs...)

		valetLink.Dir = site.LocalPath

		logCmd(valetLink, c.UI, true)
		err := valetLink.Run()

		if err != nil {
			c.UI.Error(fmt.Sprintf("Error running valet link: %s", err))
			return 1
		}

		c.UI.Info(color.GreenString(fmt.Sprintf("✓ Site %s linked\n", key)))
	}

	return 0
}

func (c *ValetLinkCommand) Synopsis() string {
	return "Link the local_path directories to Valet"
}

func (c *ValetLinkCommand) Help() string {
	helpText := `
Usage: trellis valet link [options] [ENVIRONMENT=development]

Link the local_path directories to Valet

See https://laravel.com/docs/master/valet#the-link-command for more information.

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
