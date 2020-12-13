package plugin

import (
	"github.com/mitchellh/cli"
	"reflect"
	"strings"
	"github.com/roots/trellis-cli/cmd"
)

func Register(c *cli.CLI, searchPaths []string, validPluginFilenamePrefixes []string) {
	coreRootCommands := rootCommandsFor(reflect.ValueOf(c.Commands))

	pluginFinder := finder{
		validPrefixes:    validPluginFilenamePrefixes,
		searchPaths:      searchPaths,
		coreRootCommands: coreRootCommands,
	}
	plugins := pluginFinder.find()
	pluginRootCommands := rootCommandsFor(reflect.ValueOf(plugins))

	// Register plugin commands.
	for name, bin := range plugins {
		pluginCommand := &cmd.PassthroughCommand{
			Name: name,
			Bin:  bin,
			Args: c.Args,
		}
		c.Commands[name] = func() (cli.Command, error) {
			return pluginCommand, nil
		}
	}

	// Separate plugin commands from core command lists.
	c.HiddenCommands = append(c.HiddenCommands, pluginRootCommands...)
	// Append plugin command list to help text.
	c.HelpFunc = helpFunc(c.Name, pluginRootCommands)
}

func rootCommandsFor(v reflect.Value) (rootCommands []string) {
	m := v.MapKeys()

	for _, v := range m {
		rootCommand := strings.Split(v.String(), " ")[0]
		rootCommands = append(rootCommands, rootCommand)
	}

	return unique(rootCommands)
}

// unique de-duplicates a given slice of strings without
// sorting or otherwise altering its order in any way.
func unique(values []string) (newValues []string) {
	if len(values) == 0 {
		return []string{}
	}

	seen := map[string]bool{}
	for _, v := range values {
		if seen[v] {
			continue
		}
		seen[v] = true
		newValues = append(newValues, v)
	}

	return
}
