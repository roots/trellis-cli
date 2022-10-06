package trellis

import (
	"os"
	"path/filepath"
)

type Detector interface {
	Detect(path string) (projectPath string, ok bool)
}

type ProjectDetector struct{}

/*
Detect if a path is a Trellis project or not
This will traverse up the directory tree until it finds a valid project,
or stop at the root and give up.
*/
func (p *ProjectDetector) Detect(path string) (projectPath string, ok bool) {
	configPaths, _ := filepath.Glob(filepath.Join(path, GlobPattern))

	if len(configPaths) > 0 {
		return path, true
	}

	trellisPath, ok := p.detectTrellisCLIProject(path)
	if ok {
		return trellisPath, true
	}

	parent := filepath.Dir(path)

	if len(parent) == 1 && (parent == "." || os.IsPathSeparator(parent[0])) {
		return "", false
	}

	return p.Detect(parent)
}

func (p *ProjectDetector) detectTrellisCLIProject(path string) (trellisPath string, ok bool) {
	trellisPath = filepath.Join(path, "trellis")
	configPath := filepath.Join(trellisPath, ConfigDir)

	trellisDir, err := os.Stat(trellisPath)
	if err != nil {
		return "", false
	}

	configDir, err := os.Stat(configPath)
	if err != nil {
		return "", false
	}

	if trellisDir.Mode().IsDir() && configDir.Mode().IsDir() {
		return trellisPath, true
	}

	return "", false
}
