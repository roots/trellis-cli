package cmd

import (
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type OpenCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *OpenCommand) Run(args []string) int {
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

	var openArgs = []string{}

	if len(args) == 0 {
		_, site, siteErr := c.Trellis.MainSiteFromEnvironment("development")
		if siteErr != nil {
			c.UI.Error(siteErr.Error())
			return 1
		}

		openArgs = []string{site.MainUrl()}
	} else {
		value, exists := c.Trellis.CliConfig.Open[args[0]]

		if !exists {
			c.UI.Error(fmt.Sprintf("Error: shortcut '%s' does not exist. Check your .trellis/cli.yml config file.", args[0]))
			c.UI.Error(fmt.Sprintf("Valid shortcuts are: %s", strings.Join(openNames(c.Trellis.CliConfig.Open), ", ")))
			return 1
		}

		openArgs = []string{value}
	}

	openCommandName := OpenCommandName()

	if openCommandName == "" {
		c.UI.Error("Error: open command only supported on macOS and Linux")
		return 1
	}

	open := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(c.UI),
	).Cmd(openCommandName, openArgs)

	err := open.Run()

	if err != nil {
		c.UI.Error(fmt.Sprintf("Error running open: %s", err))
		return 1
	}

	return 0
}

func (c *OpenCommand) Synopsis() string {
	return "Opens user-defined URLs (and more) which can act as shortcuts/bookmarks specific to your Trellis projects."
}

func (c *OpenCommand) Help() string {
	helpText := `
Usage: trellis open [options] [NAME]

Opens user-defined URLs (and more) which can act as shortcuts/bookmarks specific to your Trellis projects.

Without any arguments provided, this defaults to opening the main site's development canonical URL.

Additional entries can be customized by adding them to your project's config file (in '.trellis/cli.yml'):

  open:
    sentry: https://myapp.sentry.io
    newrelic: https://myapp.newrelic.com

Opens the main site's canonical URL in your web browser:

  $ trellis open

Opens the 'newrelic' shortcut (defined in config):

  $ trellis open newrelic

Arguments:
  NAME Name of shortcut

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}

func (c *OpenCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictFunc(func(args complete.Args) []string {
		if err := c.Trellis.LoadProject(); err != nil {
			return []string{}
		}

		switch len(args.Completed) {
		case 0:
			return openNames(c.Trellis.CliConfig.Open)
		default:
			return []string{}
		}
	})
}

func (c *OpenCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}

func openNames(config map[string]string) []string {
	var names []string

	for key := range config {
		names = append(names, key)
	}

	sort.Strings(names)
	return names
}

func OpenCommandName() (commandName string) {
	switch runtime.GOOS {
	case "darwin":
		commandName = "open"
	case "linux":
		commandName = "xdg-open"
	}

	return commandName
}
