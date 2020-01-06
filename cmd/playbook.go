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
	// TODO: Panic if root & ui are empty.
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
