package cmd

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

type AdHocPlaybook struct {
	Playbook
	files map[string]string
}

func (p *AdHocPlaybook) Run(playbookYml string, args []string) error {
	// TODO: Panic if files is empty.
	defer p.removeFiles()
	if err := p.dumpFiles(); err != nil {
		return err
	}

	return p.Playbook.Run(playbookYml, args)
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
