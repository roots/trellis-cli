package cmd

import (
	"fmt"
	"strings"

	"github.com/pterm/pterm"
)

type NamespaceCommand struct {
	SynopsisText string
	HelpText     string
	Subcommands  map[string]string // name -> description mapping
}

func (c *NamespaceCommand) Run(args []string) int {
	// Suppress subcommand help when showing namespace help
	setSuppressPtermOutput(true)
	defer setSuppressPtermOutput(false)

	// Always show help for namespace commands
	// Don't return cli.RunResultHelp as it causes subcommand help to show
	fmt.Print(c.Help())
	return 0
}

func (c *NamespaceCommand) Synopsis() string {
	return c.SynopsisText
}

func (c *NamespaceCommand) Help() string {
	// Define color scheme
	dim := pterm.NewStyle(pterm.FgDarkGray)
	cyan := pterm.NewStyle(pterm.FgCyan)
	green := pterm.NewStyle(pterm.FgGreen)
	brightWhite := pterm.NewStyle(pterm.FgLightWhite, pterm.Bold)

	// Parse the namespace from HelpText (e.g., "Usage: trellis db <subcommand>")
	lines := strings.Split(c.HelpText, "\n")
	var namespaceName string
	if len(lines) > 0 && strings.HasPrefix(lines[0], "Usage: trellis ") {
		parts := strings.Fields(lines[0])
		if len(parts) >= 3 {
			namespaceName = parts[2]
		}
	}

	// Build styled output as a string
	var output strings.Builder

	output.WriteString("\n")
	output.WriteString(cyan.Sprint("┌─╼ "))
	output.WriteString(brightWhite.Sprint("trellis " + namespaceName))
	output.WriteString("\n")
	output.WriteString(cyan.Sprint("└─╼ "))
	output.WriteString(dim.Sprint(c.SynopsisText))
	output.WriteString("\n\n")

	// Print usage
	if len(lines) > 0 {
		usageLine := strings.TrimPrefix(lines[0], "Usage: ")
		output.WriteString(dim.Sprint("$ "))
		output.WriteString(usageLine)
		output.WriteString("\n\n")
	}

	// Print subcommands section
	output.WriteString(" ")
	output.WriteString(cyan.Sprint("◉ "))
	output.WriteString(dim.Sprint("SUBCOMMANDS"))
	output.WriteString("\n\n")

	// Display subcommands from the Subcommands map
	if len(c.Subcommands) > 0 {
		for cmdName, cmdDesc := range c.Subcommands {
			output.WriteString(fmt.Sprintf("   %s %-15s %s\n",
				green.Sprint("→"),
				cmdName,
				dim.Sprint(cmdDesc)))
		}
	}

	output.WriteString("\n")
	return output.String()
}
