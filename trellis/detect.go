package trellis

import (
	"os"
	"path/filepath"
)

type Detector interface {
	Detect(path string) (projectPath string, ok bool)
}

type Project struct{}

const GlobPattern = "group_vars/*/wordpress_sites.yml"

/*
Detect if a path is a Trellis project or not
This will traverse up the directory tree until it finds a valid project,
or stop at the root and give up.
*/
func (p *Project) Detect(path string) (projectPath string, ok bool) {
	configPaths, _ := filepath.Glob(filepath.Join(path, GlobPattern))

	if len(configPaths) == 0 {
		if p.detectTrellisCLIProject(path) {
			return filepath.Join(path, "trellis"), true
		}

		parent := filepath.Dir(path)

		if len(parent) == 1 && (parent == "." || os.IsPathSeparator(parent[0])) {
			return "", false
		}

		return p.Detect(parent)
	}

	return path, true
}

func (p *Project) detectTrellisCLIProject(path string) bool {
	trellisPath := filepath.Join(path, "trellis")
	sitePath := filepath.Join(path, "site")
	configPath := filepath.Join(trellisPath, ConfigDir)

	trellisDir, err := os.Stat(trellisPath)
	if err != nil {
		return false
	}

	configDir, err := os.Stat(configPath)
	if err != nil {
		return false
	}

	siteDir, err := os.Stat(sitePath)
	if err != nil {
		return false
	}

	if trellisDir.Mode().IsDir() && siteDir.Mode().IsDir() && configDir.Mode().IsDir() {
		return true
	}

	return false
}
