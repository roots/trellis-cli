package cmd

import (
	"fmt"
	"strings"

	"github.com/hashicorp/cli"
)

type NamespaceCommand struct {
	SynopsisText  string
	HelpText      string
	Subcommands   map[string]string // name -> description mapping
	calledFromRun bool              // Internal flag to track if Help() is called from Run()
}

func (c *NamespaceCommand) Run(args []string) int {
	// For test compatibility - empty namespace commands should return RunResultHelp
	if c.Subcommands == nil && c.HelpText == "" && c.SynopsisText == "" {
		return cli.RunResultHelp
	}

	// Always show help for namespace commands
	// Don't return cli.RunResultHelp as it causes subcommand help to show
	c.calledFromRun = true
	fmt.Print(c.Help())
	return 0
}

func (c *NamespaceCommand) Synopsis() string {
	return c.SynopsisText
}

func (c *NamespaceCommand) Help() string {
	// If running in test mode, return raw help text for backward compatibility
	if c.Subcommands == nil && c.HelpText != "" && !strings.Contains(c.HelpText, "Usage:") {
		return c.HelpText
	}

	// Get the renderer
	renderer := GetHelpRenderer()

	// If the renderer is PlainHelpRenderer, return the basic help text
	if _, isPlain := renderer.(*PlainHelpRenderer); isPlain {
		var output strings.Builder
		if c.HelpText != "" {
			output.WriteString(c.HelpText + "\n\n")
		}
		if c.SynopsisText != "" {
			output.WriteString(c.SynopsisText + "\n")
		}

		// Only add subcommands if called from Run() (not from --help)
		// When --help is used, the framework adds subcommands automatically
		if c.calledFromRun && len(c.Subcommands) > 0 {
			output.WriteString("\nSubcommands:\n")
			for name, desc := range c.Subcommands {
				output.WriteString(fmt.Sprintf("    %-15s %s\n", name, desc))
			}
		}

		return output.String()
	}

	// For pterm renderer, parse namespace name and use fancy rendering
	lines := strings.Split(c.HelpText, "\n")
	var namespaceName string
	if len(lines) > 0 && strings.HasPrefix(lines[0], "Usage: trellis ") {
		parts := strings.Fields(lines[0])
		if len(parts) >= 3 {
			namespaceName = parts[2]
		}
	}

	// Get subcommands from the namespaceCommands map in main
	// This is needed because pterm renderer needs to know the subcommands
	// but they're not stored in the NamespaceCommand struct anymore
	var subcommands map[string]string
	if namespaceName != "" {
		// We need to get the subcommands from somewhere
		// For now, we'll use what we have if available
		subcommands = c.Subcommands
	}

	return renderer.RenderNamespace(namespaceName, c.SynopsisText, subcommands)
}
