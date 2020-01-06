package cmd

import (
	"github.com/mitchellh/cli"
	"io/ioutil"
	"os"
	"path/filepath"
)

type AdHocPlaybook struct {
	Playbook
	files map[string]string
}

func (p *AdHocPlaybook) Run(playbookYml string, args []string, ui cli.Ui) error {
	// TODO: Panic if root & files are empty.
	defer p.removeFiles()
	if err := p.dumpFiles(); err != nil {
		return err
	}

	playbook := &Playbook{}
	playbook.SetRoot(p.root)

	return playbook.Run(playbookYml, args, ui)
}

func (p *AdHocPlaybook) dumpFiles() error {
	for fileName, content := range p.files {
		destination := filepath.Join(p.root, fileName)
		contentByte := []byte(content)

		if err := ioutil.WriteFile(destination, contentByte, 0644); err != nil {
			return err
		}
	}

	return nil
}

func (p *AdHocPlaybook) removeFiles() error {
	for fileName, _ := range p.files {
		destination := filepath.Join(p.root, fileName)

		if err := os.Remove(destination); err != nil {
			return err
		}
	}

	return nil
}
