package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"plugin"
	"sort"
	"strings"
	"trellis-cli/cmd"
	"trellis-cli/trellis"

	"github.com/mitchellh/cli"
)

const Version = "0.3.1"

type PluginCommands interface {
	CommandFactory(ui cli.Ui, trellis *trellis.Trellis) map[string]cli.CommandFactory
}

func main() {
	project := &trellis.Project{}
	trellis := trellis.NewTrellis(project)

	ui := &cli.ColoredUi{
		ErrorColor: cli.UiColorRed,
		Ui: &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stderr,
		},
	}

	commands := map[string]cli.CommandFactory{
		"check": func() (cli.Command, error) {
			return &cmd.CheckCommand{UI: ui, Trellis: trellis}, nil
		},
		"deploy": func() (cli.Command, error) {
			return &cmd.DeployCommand{UI: ui, Trellis: trellis}, nil
		},
		"droplet": func() (cli.Command, error) {
			return &cmd.DropletCommand{UI: ui, Trellis: trellis}, nil
		},
		"droplet create": func() (cli.Command, error) {
			return cmd.NewDropletCreateCommand(ui, trellis), nil
		},
		"galaxy": func() (cli.Command, error) {
			return &cmd.GalaxyCommand{UI: ui, Trellis: trellis}, nil
		},
		"galaxy install": func() (cli.Command, error) {
			return &cmd.GalaxyInstallCommand{UI: ui, Trellis: trellis}, nil
		},
		"info": func() (cli.Command, error) {
			return &cmd.InfoCommand{UI: ui, Trellis: trellis}, nil
		},
		"new": func() (cli.Command, error) {
			return cmd.NewNewCommand(ui, trellis, Version), nil
		},
		"provision": func() (cli.Command, error) {
			return cmd.NewProvisionCommand(ui, trellis), nil
		},
		"rollback": func() (cli.Command, error) {
			return cmd.NewRollbackCommand(ui, trellis), nil
		},
		"vault": func() (cli.Command, error) {
			return &cmd.VaultCommand{UI: ui, Trellis: trellis}, nil
		},
		"vault edit": func() (cli.Command, error) {
			return cmd.NewVaultEditCommand(ui, trellis), nil
		},
		"vault encrypt": func() (cli.Command, error) {
			return cmd.NewVaultEncryptCommand(ui, trellis), nil
		},
		"vault decrypt": func() (cli.Command, error) {
			return cmd.NewVaultDecryptCommand(ui, trellis), nil
		},
		"vault view": func() (cli.Command, error) {
			return cmd.NewVaultViewCommand(ui, trellis), nil
		},
	}

	pluginCommands := loadPlugins(ui, trellis)

	for name, cmdPlugin := range pluginCommands {
		commands[name] = cmdPlugin
	}

	c := &cli.CLI{
		Name:         "trellis",
		Version:      Version,
		Autocomplete: true,
		HelpFunc:     GroupedHelpFunc("trellis", pluginCommands),
		Commands:     commands,
		Args:         os.Args[1:],
	}

	exitStatus, err := c.Run()

	if err != nil {
		ui.Error(err.Error())
	}

	os.Exit(exitStatus)
}

func GroupedHelpFunc(app string, pluginCommands map[string]cli.CommandFactory) cli.HelpFunc {
	return func(commands map[string]cli.CommandFactory) string {
		var buf bytes.Buffer

		// Filter out plugin commands
		for name, _ := range commands {
			_, ok := pluginCommands[name]
			if ok {
				delete(commands, name)
			}
		}

		buf.WriteString(fmt.Sprintf(
			"Usage: %s [--version] [--help] <command> [<args>]\n\n",
			app))
		buf.WriteString("Available commands are:\n")
		printCommand(&buf, commands)

		// plugin commands
		buf.WriteString("\nCommands from plugins:\n")
		printCommand(&buf, pluginCommands)

		return buf.String()
	}
}

func printCommand(buf *bytes.Buffer, commands map[string]cli.CommandFactory) *bytes.Buffer {
	keys := make([]string, 0, len(commands))
	maxKeyLen := 0
	for key := range commands {
		if len(key) > maxKeyLen {
			maxKeyLen = len(key)
		}

		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		commandFunc, ok := commands[key]
		if !ok {
			panic("command not found: " + key)
		}

		command, err := commandFunc()
		if err != nil {
			log.Printf("[ERR] cli: Command '%s' failed to load: %s",
				key, err)
			continue
		}

		key = fmt.Sprintf("%s%s", key, strings.Repeat(" ", maxKeyLen-len(key)))
		buf.WriteString(fmt.Sprintf("    %s    %s\n", key, command.Synopsis()))
	}

	return buf
}

func loadPlugins(ui cli.Ui, trellis *trellis.Trellis) map[string]cli.CommandFactory {
	plugins, _ := filepath.Glob("*_command.so")
	pluginCommands := make(map[string]cli.CommandFactory, len(plugins))

	for _, cmdPlugin := range plugins {
		plug, err := plugin.Open(cmdPlugin)
		if err != nil {
			ui.Error(fmt.Sprintf("Failed to open plugin %s: %v\n", cmdPlugin, err))
			continue
		}

		cmdSymbol, err := plug.Lookup("Commands")
		if err != nil {
			ui.Error(fmt.Sprintf("Failed to load plugin %s: plugin does not export required 'Commands' symbol\n", cmdPlugin))
			continue
		}
		commands, ok := cmdSymbol.(PluginCommands)
		if !ok {
			ui.Error(fmt.Sprintf("Failed to load plugin %s: plugin does not implement 'PluginCommands' interface\n", cmdPlugin))
			continue
		}

		for name, cmd := range commands.CommandFactory(ui, trellis) {
			pluginCommands[name] = cmd
		}
	}

	return pluginCommands
}
