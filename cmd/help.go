package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/pterm/pterm"
	"golang.org/x/term"
)

// CreateHelp is a helper function for commands to get properly formatted help
func CreateHelp(commandName string, synopsis string, rawHelp string) string {
	renderer := GetHelpRenderer()
	return renderer.RenderCommand(commandName, synopsis, rawHelp)
}

// PtermHelpFunc creates a stylized help output for subcommands
func PtermHelpFunc(commandName string, synopsis string, helpText string) {
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

	// Build output as string instead of printing directly
	var output strings.Builder

	output.WriteString("\n")
	output.WriteString(cyan.Sprint("┌─╼ "))
	output.WriteString(brightWhite.Sprint("trellis " + commandName))
	output.WriteString("\n")
	output.WriteString(cyan.Sprint("└─╼ "))
	output.WriteString(dim.Sprint(synopsis))
	output.WriteString("\n\n")

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
			output.WriteString(dim.Sprint("$ "))
			output.WriteString(strings.TrimSpace(usageLine))
			output.WriteString("\n\n")
			continue
		} else if strings.HasPrefix(trimmed, "Arguments:") {
			currentSection = "arguments"
			continue
		} else if strings.HasPrefix(trimmed, "Options:") {
			currentSection = "options"
			continue
		} else if strings.HasPrefix(trimmed, "Create") || strings.HasPrefix(trimmed, "Specify") || strings.HasPrefix(trimmed, "Force") || strings.HasPrefix(trimmed, "$") {
			// When we hit examples, stop adding to description
			// This prevents example lead-in text from showing in description
			inExampleBlock = true
			currentSection = "examples"
			// Remove the last few description lines that are probably example lead-ins
			if strings.HasPrefix(trimmed, "$") && len(description) > 0 {
				// For db open specifically, remove ALL description since it's all example lead-ins
				// More aggressive cleanup - remove trailing description lines
				for len(description) > 0 {
					lastDesc := description[len(description)-1]
					if lastDesc == "" ||
						strings.Contains(lastDesc, ":") ||
						strings.Contains(lastDesc, "example") ||
						strings.Contains(lastDesc, "database") ||
						strings.Contains(lastDesc, "production") ||
						strings.Contains(lastDesc, "defaults to") {
						description = description[:len(description)-1]
					} else {
						break
					}
				}
			}
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
				!strings.HasPrefix(trimmed, "Force") &&
				!strings.HasPrefix(trimmed, "$") &&
				!strings.Contains(trimmed, "defaults to") &&
				!strings.Contains(trimmed, "database") &&
				!strings.Contains(trimmed, "production") &&
				!strings.Contains(trimmed, ":") {
				// This is description text after Usage
				// Skip lines that look like example lead-ins
				description = append(description, trimmed)
			}

		case "examples":
			if strings.HasPrefix(trimmed, "$") {
				examples = append(examples, trimmed)
				inExampleBlock = false
			} else if inExampleBlock && trimmed != "" {
				// Example description
				examples = append(examples, "  "+trimmed)
			} else if trimmed != "" && !strings.HasPrefix(trimmed, "Arguments:") && !strings.HasPrefix(trimmed, "Options:") {
				// Any other text in examples section that's not empty and not a section header
				examples = append(examples, trimmed)
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
				output.WriteString("\n")
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
							output.WriteString(dim.Sprint(currentLine))
							output.WriteString("\n")
						} else {
							output.WriteString(dim.Sprint("  " + currentLine)) // Indent wrapped lines
							output.WriteString("\n")
						}
						lineCount++
						currentLine = word
					}
				}
				// Print last line
				if currentLine != "" {
					if lineCount == 0 {
						output.WriteString(dim.Sprint(currentLine))
						output.WriteString("\n")
					} else {
						output.WriteString(dim.Sprint("  " + currentLine)) // Indent wrapped lines
						output.WriteString("\n")
					}
				}
			}
		}
		output.WriteString("\n")
	}

	// Print Arguments section
	if len(arguments) > 0 {
		output.WriteString(" ")
		output.WriteString(cyan.Sprint("◉ "))
		output.WriteString(dim.Sprint("ARGUMENTS"))
		output.WriteString("\n")

		for _, arg := range arguments {
			parts := strings.SplitN(strings.TrimSpace(arg), " ", 2)
			if len(parts) == 2 {
				output.WriteString(fmt.Sprintf("   %s %-12s %s\n",
					green.Sprint("→"),
					parts[0],
					dim.Sprint(strings.TrimSpace(parts[1]))))
			}
		}
		output.WriteString("\n")
	}

	// Print Options section with proper word wrapping
	if len(options) > 0 {
		output.WriteString(" ")
		output.WriteString(cyan.Sprint("◉ "))
		output.WriteString(dim.Sprint("OPTIONS"))
		output.WriteString("\n")

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
				output.WriteString(prefix)

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
								output.WriteString(dim.Sprint(currentLine))
								output.WriteString("\n")
								firstLine = false
							} else {
								output.WriteString(fmt.Sprintf("%*s%s\n", visualPrefixLen, "", dim.Sprint(currentLine)))
							}
							currentLine = word
						}
					}

					// Print last line
					if currentLine != "" {
						if firstLine {
							output.WriteString(dim.Sprint(currentLine))
							output.WriteString("\n")
						} else {
							output.WriteString(fmt.Sprintf("%*s%s\n", visualPrefixLen, "", dim.Sprint(currentLine)))
						}
					}
				} else {
					output.WriteString("\n")
				}
			}
		}
		output.WriteString("\n")
	}

	// Print Examples section with consistent indentation
	if len(examples) > 0 {
		output.WriteString(" ")
		output.WriteString(cyan.Sprint("◉ "))
		output.WriteString(dim.Sprint("EXAMPLES"))
		output.WriteString("\n")

		baseIndent := "   " // 3 spaces for all example content
		lastWasCommand := false

		for _, example := range examples {
			trimmed := strings.TrimSpace(example)

			// Add blank line before description that follows a command
			if lastWasCommand && !strings.HasPrefix(trimmed, "$") {
				output.WriteString("\n")
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
								output.WriteString(baseIndent + yellow.Sprint(remaining))
								output.WriteString("\n")
							} else {
								output.WriteString(baseIndent + "  " + yellow.Sprint(remaining))
								output.WriteString("\n")
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
							output.WriteString(baseIndent + yellow.Sprint(remaining[:breakPoint]))
							output.WriteString("\n")
							firstLine = false
						} else {
							output.WriteString(baseIndent + "  " + yellow.Sprint(remaining[:breakPoint]))
							output.WriteString("\n")
						}
						remaining = strings.TrimSpace(remaining[breakPoint:])
					}
				} else {
					// Short command, print with standard indent
					output.WriteString(baseIndent + yellow.Sprint(trimmed))
					output.WriteString("\n")
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
							output.WriteString(baseIndent + dim.Sprint(currentLine))
							output.WriteString("\n")
							currentLine = word
						}
					}
					if currentLine != "" {
						output.WriteString(baseIndent + dim.Sprint(currentLine))
						output.WriteString("\n")
					}
				} else {
					output.WriteString(baseIndent + dim.Sprint(trimmedExample))
					output.WriteString("\n")
				}
			}
		}
		output.WriteString("\n")
	}

	// Footer with responsive separator
	output.WriteString(cyan.Sprint(strings.Repeat("━", termWidth)))
	output.WriteString("\n")

	// Simple footer that works at any width
	docsUrl := "https://roots.io/trellis/docs/"
	footer := " docs → " + docsUrl

	if len(footer) <= termWidth {
		output.WriteString(dim.Sprint(" docs → "))
		output.WriteString(cyan.Sprint(docsUrl))
		output.WriteString("\n")
	} else {
		// For very narrow terminals, just show the URL
		output.WriteString(cyan.Sprint(" " + docsUrl))
		output.WriteString("\n")
	}
	fmt.Print(output.String())
}
