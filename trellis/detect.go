package trellis

import (
	"os"
	"path/filepath"
)

type Detector interface {
	Detect(path string) (projectPath string, ok bool)
}

type Project struct{}

/*
Detect if a path is a Trellis project or not
This will traverse up the directory tree until it finds a valid project,
or stop at the root and give up.
*/
func (p *Project) Detect(path string) (projectPath string, ok bool) {
	configPaths, _ := filepath.Glob(filepath.Join(path, "group_vars/*/wordpress_sites.yml"))

	if len(configPaths) == 0 {
		parent := filepath.Dir(path)

		if len(parent) == 1 && (parent == "." || os.IsPathSeparator(parent[0])) {
			return "", false
		}

		return p.Detect(parent)
	}

	return path, true
}
