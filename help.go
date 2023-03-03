package main

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/mitchellh/cli"
)

func experimentalCommandHelpFunc(app string, f cli.HelpFunc) cli.HelpFunc {
	return func(commands map[string]cli.CommandFactory) string {
		var buf bytes.Buffer
		if len(experimentalCommands) > 0 {
			buf.WriteString("\n\nExperimental commands:\n")
		}

		maxKeyLen := 0
		keys := make([]string, 0, len(experimentalCommands))
		filteredCommands := make(map[string]cli.CommandFactory)

		for key, command := range commands {
			for _, experimentalKey := range experimentalCommands {
				if key != experimentalKey {
					filteredCommands[key] = command
				}
			}
		}

		for _, key := range experimentalCommands {
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
