package plugin

import (
	"bytes"
	"fmt"
	"github.com/mitchellh/cli"
)

func helpFunc(app string, pluginRootCommands []string) cli.HelpFunc {
	return func(commands map[string]cli.CommandFactory) string {
		var buf bytes.Buffer
		if len(pluginRootCommands) > 0 {
			buf.WriteString("\n\nAvailable third party plugin commands are:\n")
		}

		for _, p := range pluginRootCommands {
			buf.WriteString(fmt.Sprintf("    %s\n", p))
		}

		return cli.BasicHelpFunc(app)(commands) + buf.String()
	}
}
