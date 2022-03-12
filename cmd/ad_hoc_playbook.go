package cmd

import (
	"os"
	"path/filepath"
)

type AdHocPlaybook struct {
	files map[string]string
	path  string
}

func (p *AdHocPlaybook) DumpFiles() func() {
	for fileName, content := range p.files {
		destination := filepath.Join(p.path, fileName)
		contentByte := []byte(content)

		if err := os.WriteFile(destination, contentByte, 0644); err != nil {
			panic("Could not write temporary file. This is probably a bug in trellis-cli; please open an issue to let us know.")
		}
	}

	return func() {
		p.removeFiles()
	}
}

func (p *AdHocPlaybook) removeFiles() {
	for fileName := range p.files {
		destination := filepath.Join(p.path, fileName)

		if err := os.Remove(destination); err != nil {
			panic("Could not delete temporary file. This is probably a bug in trellis-cli; please open an issue to let us know.")
		}
	}
}
