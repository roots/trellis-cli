package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// ptermHelpFunc creates a minimal, modern help function using pterm
func ptermHelpFunc(version string, deprecatedCommands []string, baseHelp cli.HelpFunc) cli.HelpFunc {
	return func(commands map[string]cli.CommandFactory) string {
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

		// Check if command is deprecated
		isDeprecated := func(name string) bool {
			for _, d := range deprecatedCommands {
				if d == name {
					return true
				}
			}
			return false
		}

		// Categorize commands (skip deprecated)
		for name, factory := range commands {
			// Skip sub-commands and deprecated
			if strings.Contains(name, " ") || isDeprecated(name) {
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
			for _, cmd := range cmds {
				// Build the prefix: "   → command       "
				prefix := fmt.Sprintf("   %s %-14s ", green.Sprint("→"), cmd.name)

				// Word wrap with exact visual length of prefix
				// The arrow takes 1 visual space even though it might be multi-byte
				visualPrefixLen := 3 + 1 + 1 + 14 + 1 // spaces + arrow + space + name + space = 20

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
}
