package cmd

import (
	"fmt"
	"strings"

	"github.com/pterm/pterm"
	"golang.org/x/term"
	"os"
)

// CreateHelp is a helper function for commands to get properly formatted help
func CreateHelp(commandName string, synopsis string, rawHelp string) string {
	return PtermHelpFunc(commandName, synopsis, rawHelp)
}

// PtermHelpFunc creates a stylized help output for subcommands
func PtermHelpFunc(commandName string, synopsis string, helpText string) string {
	// Define color scheme
	dim := pterm.NewStyle(pterm.FgDarkGray)
	cyan := pterm.NewStyle(pterm.FgCyan)
	green := pterm.NewStyle(pterm.FgGreen)
	brightWhite := pterm.NewStyle(pterm.FgLightWhite, pterm.Bold)
	yellow := pterm.NewStyle(pterm.FgYellow)

	// Get terminal width
	termWidth := 80
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		termWidth = width
	}

	// Clear for clean output
	fmt.Println()

	// Command header - minimal style
	fmt.Print(cyan.Sprint("┌─╼ "))
	fmt.Print(brightWhite.Sprint("trellis " + commandName))
	fmt.Println()
	fmt.Print(cyan.Sprint("└─╼ "))
	fmt.Println(dim.Sprint(synopsis))
	fmt.Println()

	// Parse the help text to extract usage, examples, arguments, and options
	lines := strings.Split(helpText, "\n")

	var currentSection string
	var examples []string
	var arguments []string
	var options []string
	var description []string

	inExampleBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Detect sections
		if strings.HasPrefix(line, "Usage:") {
			currentSection = "usage"
			// Extract and print usage immediately
			usageLine := strings.TrimPrefix(line, "Usage:")
			fmt.Print(dim.Sprint("$ "))
			fmt.Println(strings.TrimSpace(usageLine))
			fmt.Println()
			continue
		} else if strings.HasPrefix(trimmed, "Arguments:") {
			currentSection = "arguments"
			continue
		} else if strings.HasPrefix(trimmed, "Options:") {
			currentSection = "options"
			continue
		} else if strings.HasPrefix(trimmed, "Create") || strings.HasPrefix(trimmed, "Specify") || strings.HasPrefix(trimmed, "Force") {
			inExampleBlock = true
			currentSection = "examples"
		}

		// Collect content based on section
		switch currentSection {
		case "usage":
			// After usage line, collect description until we hit Arguments/Options/Examples
			if trimmed == "" {
				// Preserve blank lines in description
				if len(description) > 0 && description[len(description)-1] != "" {
					description = append(description, "")
				}
			} else if !strings.HasPrefix(trimmed, "Arguments:") &&
				!strings.HasPrefix(trimmed, "Options:") &&
				!strings.HasPrefix(trimmed, "Create") &&
				!strings.HasPrefix(trimmed, "Specify") &&
				!strings.HasPrefix(trimmed, "Force") {
				// This is description text after Usage
				description = append(description, trimmed)
			}

		case "examples":
			if strings.HasPrefix(trimmed, "$") {
				examples = append(examples, trimmed)
				inExampleBlock = false
			} else if inExampleBlock {
				// Example description
				examples = append(examples, "  "+trimmed)
			}

		case "arguments":
			if strings.Contains(line, "  ") && !strings.HasPrefix(trimmed, "Options:") {
				arguments = append(arguments, line)
			}

		case "options":
			if strings.Contains(line, "  ") || strings.HasPrefix(line, "      ") {
				options = append(options, line)
			}
		}
	}

	// Print description with manual word wrapping for better control
	if len(description) > 0 {
		for _, desc := range description {
			if desc == "" {
				// Preserve blank lines
				fmt.Println()
			} else {
				// Manual word wrapping with proper indentation
				words := strings.Fields(desc)
				currentLine := ""
				lineCount := 0
				maxWidth := termWidth - 4 // Leave some margin

				for _, word := range words {
					if currentLine == "" {
						currentLine = word
					} else if len(currentLine)+1+len(word) <= maxWidth {
						currentLine += " " + word
					} else {
						// Print current line
						if lineCount == 0 {
							fmt.Println(dim.Sprint(currentLine))
						} else {
							fmt.Println(dim.Sprint("  " + currentLine)) // Indent wrapped lines
						}
						lineCount++
						currentLine = word
					}
				}
				// Print last line
				if currentLine != "" {
					if lineCount == 0 {
						fmt.Println(dim.Sprint(currentLine))
					} else {
						fmt.Println(dim.Sprint("  " + currentLine)) // Indent wrapped lines
					}
				}
			}
		}
		fmt.Println()
	}

	// Print Arguments section
	if len(arguments) > 0 {
		fmt.Print(" ")
		fmt.Print(cyan.Sprint("◉ "))
		fmt.Println(dim.Sprint("ARGUMENTS"))

		for _, arg := range arguments {
			parts := strings.SplitN(strings.TrimSpace(arg), " ", 2)
			if len(parts) == 2 {
				fmt.Printf("   %s %-12s %s\n",
					green.Sprint("→"),
					parts[0],
					dim.Sprint(strings.TrimSpace(parts[1])))
			}
		}
		fmt.Println()
	}

	// Print Options section with proper word wrapping
	if len(options) > 0 {
		fmt.Print(" ")
		fmt.Print(cyan.Sprint("◉ "))
		fmt.Println(dim.Sprint("OPTIONS"))

		for _, opt := range options {
			trimmed := strings.TrimSpace(opt)
			if trimmed == "" {
				continue
			}

			// Handle option lines
			if strings.HasPrefix(trimmed, "--") || strings.HasPrefix(trimmed, "-") {
				// Find the description part (after multiple spaces)
				flagPart := ""
				descPart := ""

				// Look for two or more spaces to find where description starts
				if idx := strings.Index(trimmed, "  "); idx != -1 {
					flagPart = strings.TrimSpace(trimmed[:idx])
					descPart = strings.TrimSpace(trimmed[idx:])
				} else {
					flagPart = trimmed
				}

				// Build the prefix with arrow and flag
				prefix := fmt.Sprintf("   %s %-20s ", green.Sprint("→"), flagPart)

				// Calculate visual length for proper alignment
				visualPrefixLen := 3 + 1 + 1 + 20 + 1 // "   " + arrow + " " + flag + " " = 26

				// Print first line with prefix
				fmt.Print(prefix)

				if descPart != "" {
					// Word wrap the description based on terminal width
					descWidth := termWidth - visualPrefixLen

					words := strings.Fields(descPart)
					currentLine := ""
					firstLine := true

					for _, word := range words {
						if currentLine == "" {
							currentLine = word
						} else if len(currentLine)+1+len(word) <= descWidth {
							currentLine += " " + word
						} else {
							// Print current line
							if firstLine {
								fmt.Println(dim.Sprint(currentLine))
								firstLine = false
							} else {
								fmt.Printf("%*s%s\n", visualPrefixLen, "", dim.Sprint(currentLine))
							}
							currentLine = word
						}
					}

					// Print last line
					if currentLine != "" {
						if firstLine {
							fmt.Println(dim.Sprint(currentLine))
						} else {
							fmt.Printf("%*s%s\n", visualPrefixLen, "", dim.Sprint(currentLine))
						}
					}
				} else {
					fmt.Println()
				}
			}
		}
		fmt.Println()
	}

	// Print Examples section with consistent indentation
	if len(examples) > 0 {
		fmt.Print(" ")
		fmt.Print(cyan.Sprint("◉ "))
		fmt.Println(dim.Sprint("EXAMPLES"))

		baseIndent := "   " // 3 spaces for all example content
		lastWasCommand := false

		for _, example := range examples {
			trimmed := strings.TrimSpace(example)

			// Add blank line before description that follows a command
			if lastWasCommand && !strings.HasPrefix(trimmed, "$") {
				fmt.Println()
			}

			if strings.HasPrefix(trimmed, "$") {
				lastWasCommand = true
				// Command example - always indent by baseIndent
				maxCmdWidth := termWidth - len(baseIndent)
				if len(trimmed) > maxCmdWidth {
					// Need to wrap the command
					remaining := trimmed
					firstLine := true
					for len(remaining) > 0 {
						cutAt := maxCmdWidth
						if !firstLine {
							cutAt = maxCmdWidth - 2 // Account for continuation indent
						}

						if len(remaining) <= cutAt {
							if firstLine {
								fmt.Println(baseIndent + yellow.Sprint(remaining))
							} else {
								fmt.Println(baseIndent + "  " + yellow.Sprint(remaining))
							}
							break
						}

						// Find a good break point (space or slash)
						breakPoint := cutAt
						for i := cutAt; i > cutAt-20 && i > 0; i-- {
							if remaining[i] == ' ' || remaining[i] == '/' {
								breakPoint = i
								break
							}
						}

						if firstLine {
							fmt.Println(baseIndent + yellow.Sprint(remaining[:breakPoint]))
							firstLine = false
						} else {
							fmt.Println(baseIndent + "  " + yellow.Sprint(remaining[:breakPoint]))
						}
						remaining = strings.TrimSpace(remaining[breakPoint:])
					}
				} else {
					// Short command, print with standard indent
					fmt.Println(baseIndent + yellow.Sprint(trimmed))
				}
			} else {
				lastWasCommand = false
				// Description line - indent all description text
				maxDescWidth := termWidth - len(baseIndent)
				trimmedExample := strings.TrimSpace(example)

				if len(trimmedExample) > maxDescWidth {
					// Need to wrap
					words := strings.Fields(trimmedExample)
					currentLine := ""
					for _, word := range words {
						if currentLine == "" {
							currentLine = word
						} else if len(currentLine)+1+len(word) <= maxDescWidth {
							currentLine += " " + word
						} else {
							fmt.Println(baseIndent + dim.Sprint(currentLine))
							currentLine = word
						}
					}
					if currentLine != "" {
						fmt.Println(baseIndent + dim.Sprint(currentLine))
					}
				} else {
					fmt.Println(baseIndent + dim.Sprint(trimmedExample))
				}
			}
		}
		fmt.Println()
	}

	// Footer with responsive separator
	fmt.Println(cyan.Sprint(strings.Repeat("━", termWidth)))

	// Simple footer that works at any width
	docsUrl := "https://roots.io/trellis/docs/"
	footer := " docs → " + docsUrl

	if len(footer) <= termWidth {
		fmt.Print(dim.Sprint(" docs → "))
		fmt.Println(cyan.Sprint(docsUrl))
	} else {
		// For very narrow terminals, just show the URL
		fmt.Println(cyan.Sprint(" " + docsUrl))
	}
	fmt.Println()

	return ""
}
