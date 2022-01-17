package cmd

import (
	"fmt"
	"strings"

	"github.com/fatih/color"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
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
	}

	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	config, _ := c.Trellis.Environments[environment]

	c.UI.Info(fmt.Sprintf("Linking environment %s...", environment))

	for key, site := range config.WordPressSites {
		c.UI.Info(fmt.Sprintf("Linking site %s...\n", key))

		canonical, _ := c.Trellis.HostsFromDomain(site.SiteHosts[0].Canonical, environment)
		app := strings.TrimSuffix(canonical.String(), "."+canonical.TLD)

		valetArgs := []string{"link"}

		if site.SslEnabled() {
			valetArgs = append(valetArgs, "--secure")
		}
		valetArgs = append(valetArgs, app)

		valetLink := command.WithOptions(command.WithTermOutput(), command.WithLogging(c.UI)).Cmd("valet", valetArgs)
		valetLink.Dir = site.LocalPath

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
