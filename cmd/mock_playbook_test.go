package cmd

import (
	"github.com/mitchellh/cli"
	"strings"
)

type MockPlaybook struct {
	ui   cli.Ui
}

func (p *MockPlaybook) SetRoot(root string) {
}

func (p *MockPlaybook) Run(playbookYml string, args []string) error {
	p.ui.Info("Running command => ansible-playbook " + playbookYml + " " + strings.Join(args, " "))

	return nil
}
