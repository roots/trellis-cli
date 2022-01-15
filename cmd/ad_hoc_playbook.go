package cmd

import (
	"io/ioutil"
	"os"
	"path/filepath"
)

type AdHocPlaybook struct {
	files map[string]string
	path  string
}

func (p *AdHocPlaybook) DumpFiles() error {
	for fileName, content := range p.files {
		destination := filepath.Join(p.path, fileName)
		contentByte := []byte(content)

		if err := ioutil.WriteFile(destination, contentByte, 0644); err != nil {
			return err
		}
	}

	return nil
}

func (p *AdHocPlaybook) RemoveFiles() error {
	for fileName, _ := range p.files {
		destination := filepath.Join(p.path, fileName)

		if err := os.Remove(destination); err != nil {
			return err
		}
	}

	return nil
}
