package cmd

import (
	"github.com/mitchellh/cli"
	"strings"
)

type MockPlaybook struct {
	ui       cli.Ui
	commands []string
}

func (p *MockPlaybook) SetRoot(root string) {
}

func (p *MockPlaybook) Run(playbookYml string, args []string) error {
	command := "ansible-playbook " + playbookYml + " " + strings.Join(args, " ")

	p.commands = append(p.commands, command)
	// For backward compatibility.
	p.ui.Info("Running command => " + command)

	return nil
}

func (p *MockPlaybook) GetCommands() []string {
	return p.commands
}
