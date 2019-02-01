package trellis

import (
	"os/exec"
	"strings"

	"github.com/mcuadros/go-version"
)

type Requirement struct {
	Name              string
	Command           string
	Url               string
	VersionConstraint string
	ExtractVersion    func(output string) string
}

type RequirementResult struct {
	Satisfied bool
	Installed bool
	Version   string
}

func (r *Requirement) IsInstalled() (path string, ok bool) {
	path, err := exec.LookPath(r.Command)
	if err != nil {
		return "", false
	}

	return path, true
}

func (r *Requirement) Check() (result RequirementResult, err error) {
	constraint := version.NewConstrainGroupFromString(r.VersionConstraint)
	path, installed := r.IsInstalled()

	if !installed {
		return RequirementResult{Satisfied: false, Installed: false}, nil
	}

	out, err := exec.Command(path, "--version").Output()
	version := strings.TrimSpace(string(out))

	if err != nil {
		return RequirementResult{}, err
	}

	if r.ExtractVersion != nil {
		version = r.ExtractVersion(version)
	}

	matched := constraint.Match(version)
	return RequirementResult{Satisfied: matched, Installed: true, Version: version}, nil
}
