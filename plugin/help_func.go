package plugin

import (
	"bytes"
	"fmt"
	"github.com/hashicorp/cli"
)

func helpFunc(pluginRootCommands []string, f cli.HelpFunc) cli.HelpFunc {
	return func(commands map[string]cli.CommandFactory) string {
		var buf bytes.Buffer
		if len(pluginRootCommands) > 0 {
			buf.WriteString("\n\nAvailable plugin commands:\n")
		}

		for _, p := range pluginRootCommands {
			buf.WriteString(fmt.Sprintf("    %s\n", p))
		}

		return f(commands) + buf.String()
	}
}
