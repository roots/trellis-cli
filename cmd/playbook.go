package cmd

import (
	"github.com/mitchellh/cli"
	"os"
)

type PlaybookRunner interface {
	SetRoot(root string)
	Run(playbookYml string, args []string) error
}

type Playbook struct {
	root string
	ui   cli.Ui
}

func (p *Playbook) SetRoot(root string) {
	p.root = root
}

func (p *Playbook) Run(playbookYml string, args []string) error {
	if p.root == "" {
		panic("Playbook root is empty; This is a flaw in the source code. Please send bug report.")
	}

	if p.ui == nil {
		panic("Playbook ui is nil; This is a flaw in the source code. Please send bug report.")
	}

	command := execCommand("ansible-playbook", append([]string{playbookYml}, args...)...)

	command.Dir = p.root

	env := os.Environ()
	// To allow mockExecCommand injects its environment variables.
	if command.Env != nil {
		env = command.Env
	}
	command.Env = append(env, "ANSIBLE_RETRY_FILES_ENABLED=false")

	logCmd(command, p.ui, true)

	return command.Run()
}
