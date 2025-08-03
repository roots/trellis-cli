package cmd

import (
	"github.com/hashicorp/cli"
	"testing"
)

func TestNamespaceCommandRun(t *testing.T) {
	expected := cli.RunResultHelp
	namespaceCommand := &NamespaceCommand{}

	actual := namespaceCommand.Run([]string{})

	if expected != actual {
		t.Errorf("expected output %d to be %d", actual, expected)
	}
}

func TestNamespaceCommandHelp(t *testing.T) {
	expected := "Help: foo bar"
	namespaceCommand := &NamespaceCommand{
		HelpText: expected,
	}

	actual := namespaceCommand.Help()

	if expected != actual {
		t.Errorf("expected output %s to be %s", actual, expected)
	}
}

func TestNamespaceCommandSynopsis(t *testing.T) {
	expected := "Synopsis: foo bar"
	namespaceCommand := &NamespaceCommand{
		SynopsisText: expected,
	}

	actual := namespaceCommand.Synopsis()

	if expected != actual {
		t.Errorf("expected output %s to be %s", actual, expected)
	}
}
