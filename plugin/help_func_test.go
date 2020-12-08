package plugin

import (
	"github.com/mitchellh/cli"
	"strings"
	"testing"
)

func TestHelpFunc(t *testing.T) {
	coreCommands := map[string]cli.CommandFactory{
		"dummy": func() (cli.Command, error) {
			return &cli.MockCommand{}, nil
		},
	}
	pluginRootCommands := []string{"foo", "bar"}

	output := helpFunc("app", pluginRootCommands)(coreCommands)

	expected := "Available third party plugin commands are"
	if !strings.Contains(output, expected) {
		t.Errorf("expected output %q to contain %q", output, expected)
	}

	for _, plugin := range pluginRootCommands {
		if !strings.Contains(output, plugin) {
			t.Errorf("expected output %q to contain %q", output, plugin)
		}
	}
}

func TestHelpFuncNoPlugin(t *testing.T) {
	coreCommands := map[string]cli.CommandFactory{
		"dummy": func() (cli.Command, error) {
			return &cli.MockCommand{}, nil
		},
	}
	pluginRootCommands := []string{}

	output := helpFunc("app", pluginRootCommands)(coreCommands)

	expected := cli.BasicHelpFunc("app")(coreCommands)
	if expected != output {
		t.Errorf("expected output %q to be excatly the same as %q", output, expected)
	}
}
