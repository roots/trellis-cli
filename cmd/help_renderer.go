package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// HelpRenderer defines the interface for rendering help output
type HelpRenderer interface {
	// RenderMain renders the main help output for all commands
	RenderMain(commands map[string]cli.CommandFactory, version string) string

	// RenderCommand renders help for a specific command
	RenderCommand(commandName string, synopsis string, helpText string) string

	// RenderNamespace renders help for a namespace command
	RenderNamespace(namespaceName string, synopsis string, subcommands map[string]string) string

	// ShouldIntercept returns true if this renderer needs to intercept help handling
	ShouldIntercept() bool
}

// GetHelpRenderer returns the appropriate help renderer based on the environment
func GetHelpRenderer() HelpRenderer {
	// Check if we're in a terminal
	isTerminal := term.IsTerminal(int(os.Stdout.Fd()))

	// For debugging: you can force pterm with TRELLIS_PTERM=1
	if os.Getenv("TRELLIS_PTERM") == "1" {
		return &PtermHelpRenderer{}
	}

	if !isTerminal {
		return &PlainHelpRenderer{}
	}
	return &PtermHelpRenderer{}
}

// PtermHelpRenderer renders beautiful help using pterm
type PtermHelpRenderer struct{}

func (r *PtermHelpRenderer) ShouldIntercept() bool {
	// Pterm renderer needs to intercept to prevent CLI framework issues
	return true
}

func (r *PtermHelpRenderer) RenderMain(commands map[string]cli.CommandFactory, version string) string {
	// Define minimal color scheme - modern terminal aesthetic
	dim := pterm.NewStyle(pterm.FgDarkGray)
	brightWhite := pterm.NewStyle(pterm.FgLightWhite, pterm.Bold)
	cyan := pterm.NewStyle(pterm.FgCyan)
	green := pterm.NewStyle(pterm.FgGreen)

	// Get terminal width
	termWidth := 80
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		termWidth = width
	}

	// Group commands by category
	categories := map[string][]struct{ name, desc string }{
		"project":  {},
		"dev":      {},
		"deploy":   {},
		"db":       {},
		"security": {},
		"utils":    {},
	}

	// Categorize commands (skip sub-commands and deprecated)
	for name, factory := range commands {
		// Skip sub-commands
		if strings.Contains(name, " ") {
			continue
		}

		cmd, _ := factory()
		if cmd == nil {
			continue
		}

		cmdInfo := struct{ name, desc string }{
			name: name,
			desc: cmd.Synopsis(),
		}

		// Categorize based on command name
		switch {
		case name == "new" || name == "init":
			categories["project"] = append(categories["project"], cmdInfo)
		case name == "up" || name == "down" || strings.HasPrefix(name, "vm") || name == "ssh" || name == "exec" || strings.HasPrefix(name, "valet"):
			categories["dev"] = append(categories["dev"], cmdInfo)
		case name == "deploy" || name == "provision" || name == "rollback":
			categories["deploy"] = append(categories["deploy"], cmdInfo)
		case strings.HasPrefix(name, "db"):
			categories["db"] = append(categories["db"], cmdInfo)
		case strings.HasPrefix(name, "vault") || strings.HasPrefix(name, "key"):
			categories["security"] = append(categories["security"], cmdInfo)
		default:
			categories["utils"] = append(categories["utils"], cmdInfo)
		}
	}

	fmt.Println()
	fmt.Println(cyan.Sprint("┌─╼ ") + brightWhite.Sprint("trellis") + dim.Sprint(" ─ ") + dim.Sprint(version))
	fmt.Println(cyan.Sprint("└─╼ ") + dim.Sprint("WordPress LEMP stack"))
	fmt.Println()
	fmt.Print(dim.Sprint("$ "))
	fmt.Print("trellis ")
	fmt.Print(green.Sprint("<command>"))
	fmt.Println(dim.Sprint(" [args]"))
	fmt.Println()

	// Display categories in order
	categoryDisplay := []struct {
		key   string
		label string
		icon  string
	}{
		{"project", "PROJECT", "◉"},
		{"dev", "DEV", "◉"},
		{"deploy", "DEPLOY", "◉"},
		{"db", "DATABASE", "◉"},
		{"security", "SECURITY", "◉"},
		{"utils", "UTILS", "◉"},
	}

	for _, cat := range categoryDisplay {
		cmds := categories[cat.key]
		if len(cmds) == 0 {
			continue
		}

		// Sort commands alphabetically
		sort.Slice(cmds, func(i, j int) bool {
			return cmds[i].name < cmds[j].name
		})

		// Category header with icon (single space before)
		fmt.Print(" ")
		fmt.Print(cyan.Sprint(cat.icon + " "))
		fmt.Println(dim.Sprint(cat.label))

		// Commands - clean indented list (with extra space before arrow)
		const commandPrefixLen = 20 // Total visual length of "   → command       "
		for _, cmd := range cmds {
			// Build the prefix: "   → command       "
			prefix := fmt.Sprintf("   %s %-14s ", green.Sprint("→"), cmd.name)

			// Word wrap with exact visual length of prefix
			visualPrefixLen := commandPrefixLen

			desc := cmd.desc
			descWidth := termWidth - visualPrefixLen

			fmt.Print(prefix)

			words := strings.Fields(desc)
			currentLine := ""
			firstLine := true

			for _, word := range words {
				if currentLine == "" {
					currentLine = word
				} else if len(currentLine)+1+len(word) <= descWidth {
					currentLine += " " + word
				} else {
					if firstLine {
						fmt.Println(dim.Sprint(currentLine))
						firstLine = false
					} else {
						fmt.Printf("%*s%s\n", visualPrefixLen, "", dim.Sprint(currentLine))
					}
					currentLine = word
				}
			}

			if currentLine != "" {
				if firstLine {
					fmt.Println(dim.Sprint(currentLine))
				} else {
					fmt.Printf("%*s%s\n", visualPrefixLen, "", dim.Sprint(currentLine))
				}
			}
		}
		fmt.Println()
	}

	// Footer with responsive separator
	fmt.Println(cyan.Sprint(strings.Repeat("━", termWidth)))

	// Calculate footer text length
	footerLeft := " need more info? → trellis <command> --help"
	footerRight := "docs → https://roots.io/trellis/docs/"
	footerSeparator := "   |   "
	totalFooterLen := len(footerLeft) + len(footerSeparator) + len(footerRight)

	if totalFooterLen <= termWidth {
		// Everything fits on one line
		fmt.Print(dim.Sprint(" need more info? → "))
		fmt.Print("trellis ")
		fmt.Print(green.Sprint("<command>"))
		fmt.Print(" --help")
		fmt.Print(dim.Sprint(footerSeparator))
		fmt.Print(dim.Sprint("docs → "))
		fmt.Println(cyan.Sprint("https://roots.io/trellis/docs/"))
	} else {
		// Split into two lines for narrow terminals
		fmt.Print(dim.Sprint(" need more info? → "))
		fmt.Print("trellis ")
		fmt.Print(green.Sprint("<command>"))
		fmt.Println(" --help")
		fmt.Print(dim.Sprint(" docs → "))
		fmt.Println(cyan.Sprint("https://roots.io/trellis/docs/"))
	}
	fmt.Println()

	return ""
}

func (r *PtermHelpRenderer) RenderCommand(commandName string, synopsis string, helpText string) string {
	// Use existing PtermHelpFunc
	PtermHelpFunc(commandName, synopsis, helpText)
	return ""
}

func (r *PtermHelpRenderer) RenderNamespace(namespaceName string, synopsis string, subcommands map[string]string) string {
	// Define color scheme
	dim := pterm.NewStyle(pterm.FgDarkGray)
	cyan := pterm.NewStyle(pterm.FgCyan)
	green := pterm.NewStyle(pterm.FgGreen)
	brightWhite := pterm.NewStyle(pterm.FgLightWhite, pterm.Bold)

	// Build styled output as a string
	var output strings.Builder

	output.WriteString("\n")
	output.WriteString(cyan.Sprint("┌─╼ "))
	output.WriteString(brightWhite.Sprint("trellis " + namespaceName))
	output.WriteString("\n")
	output.WriteString(cyan.Sprint("└─╼ "))
	output.WriteString(dim.Sprint(synopsis))
	output.WriteString("\n\n")

	// Print usage
	output.WriteString(dim.Sprint("$ "))
	output.WriteString(fmt.Sprintf("trellis %s <subcommand> [<args>]", namespaceName))
	output.WriteString("\n\n")

	// Print subcommands section
	output.WriteString(" ")
	output.WriteString(cyan.Sprint("◉ "))
	output.WriteString(dim.Sprint("SUBCOMMANDS"))
	output.WriteString("\n\n")

	// Display subcommands
	if len(subcommands) > 0 {
		for cmdName, cmdDesc := range subcommands {
			output.WriteString(fmt.Sprintf("   %s %-15s %s\n",
				green.Sprint("→"),
				cmdName,
				dim.Sprint(cmdDesc)))
		}
	}

	output.WriteString("\n")
	fmt.Print(output.String())
	return ""
}

// PlainHelpRenderer renders plain text help (for tests and non-TTY)
type PlainHelpRenderer struct{}

func (r *PlainHelpRenderer) ShouldIntercept() bool {
	// Plain renderer doesn't need to intercept - let CLI framework handle it
	return false
}

func (r *PlainHelpRenderer) RenderMain(commands map[string]cli.CommandFactory, version string) string {
	// Use the base CLI help for plain output
	return cli.BasicHelpFunc("trellis")(commands)
}

func (r *PlainHelpRenderer) RenderCommand(commandName string, synopsis string, helpText string) string {
	// Return the raw help text for plain output
	return helpText
}

func (r *PlainHelpRenderer) RenderNamespace(namespaceName string, synopsis string, subcommands map[string]string) string {
	// For plain renderer in non-TTY mode, we don't render anything here
	// The namespace command's HelpText already contains the formatted help
	// Returning empty string lets the framework handle it
	return ""
}
