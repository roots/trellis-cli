package cmd

import (
	"github.com/mitchellh/cli"
	"io/ioutil"
	"os"
	"path/filepath"
)

type PlaybookInterface interface {
	SetFiles(files map[string]string)
	SetRoot(root string)
	Run(playbookYml string, args []string, ui cli.Ui) error
}

type Playbook struct {
	files map[string]string
	root  string
}

func (p *Playbook) SetFiles(files map[string]string) {
	p.files = files
}

func (p *Playbook) SetRoot(root string) {
	p.root = root
}

func (p *Playbook) Run(playbookYml string, args []string, ui cli.Ui) error {
	// TODO: Panic if Root & Files are empty.

	defer p.removeFiles()
	dumpFilesErr := p.dumpFiles()
	if dumpFilesErr != nil {
		return dumpFilesErr
	}

	command := execCommand("ansible-playbook", append([]string{playbookYml}, args...)...)

	command.Dir = p.root

	env := os.Environ()
	// To allow mockExecCommand injects its environment variables.
	if command.Env != nil {
		env = command.Env
	}
	command.Env = append(env, "ANSIBLE_RETRY_FILES_ENABLED=false")

	logCmd(command, ui, true)

	return command.Run()
}

func (p *Playbook) dumpFiles() error {
	for fileName, content := range p.files {
		destination := filepath.Join(p.root, fileName)
		contentByte := []byte(content)

		if err := ioutil.WriteFile(destination, contentByte, 0644); err != nil {
			return err
		}
	}

	return nil
}

func (p *Playbook) removeFiles() error {
	for fileName, _ := range p.files {
		destination := filepath.Join(p.root, fileName)

		if err := os.Remove(destination); err != nil {
			return err
		}
	}

	return nil
}
