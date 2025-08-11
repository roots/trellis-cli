package main

import (
	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/cmd"
)

// ptermHelpFunc creates a help function that uses the renderer system
func ptermHelpFunc(version string, deprecatedCommands []string, baseHelp cli.HelpFunc) cli.HelpFunc {
	return func(commands map[string]cli.CommandFactory) string {
		// Use the renderer system to generate help
		renderer := cmd.GetHelpRenderer()
		return renderer.RenderMain(commands, version)
	}
}
