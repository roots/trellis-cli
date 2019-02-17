package main

import (
	"trellis-cli/trellis"

	"github.com/mitchellh/cli"
)

type testCommands struct{}

// Plugin entry point
// Must be named `Commands`
var Commands testCommands

// define command names
func (t *testCommands) CommandFactory(ui cli.Ui, trellis *trellis.Trellis) map[string]cli.CommandFactory {
	return map[string]cli.CommandFactory{
		"test": func() (cli.Command, error) {
			return &TestCommand{UI: ui, Trellis: trellis}, nil
		},
	}
}

type TestCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *TestCommand) Run(args []string) int {
	c.UI.Info("test command")
	return 0
}

func (c *TestCommand) Synopsis() string {
	return "test Synopsis"
}

func (c *TestCommand) Help() string {
	return "test help"
}
