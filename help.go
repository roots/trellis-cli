package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/mitchellh/cli"
)

func deprecatedCommandHelpFunc(commandNames []string, f cli.HelpFunc) cli.HelpFunc {
	return func(commands map[string]cli.CommandFactory) string {
		var buf bytes.Buffer
		if len(commandNames) > 0 {
			buf.WriteString("\n\nDeprecated commands:\n")
		}

		maxKeyLen := 0
		keys := make([]string, 0, len(commandNames))
		filteredCommands := make(map[string]cli.CommandFactory)

		for key, command := range commands {
			for _, deprecatedKey := range commandNames {
				if key != deprecatedKey {
					filteredCommands[key] = command
				}
			}
		}

		for _, key := range commandNames {
			if len(key) > maxKeyLen {
				maxKeyLen = len(key)
			}

			keys = append(keys, key)
		}

		sort.Strings(keys)

		for _, key := range keys {
			commandFunc, _ := commands[key]
			command, _ := commandFunc()
			key = fmt.Sprintf("%s%s", key, strings.Repeat(" ", maxKeyLen-len(key)))
			buf.WriteString(fmt.Sprintf("    %s    %s\n", key, command.Synopsis()))
		}

		return f(filteredCommands) + buf.String()
	}
}
