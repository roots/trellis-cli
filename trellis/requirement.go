package trellis

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/fatih/color"
	"github.com/mcuadros/go-version"
)

type Requirement struct {
	Name              string
	Command           string
	Url               string
	Optional          bool
	VersionConstraint string
	ExtractVersion    func(output string) string
}

type RequirementResult struct {
	Satisfied bool
	Installed bool
	Message   string
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
	message := fmt.Sprintf("%s [%s]:", r.Name, r.VersionConstraint)

	if !installed {
		return RequirementResult{
			Satisfied: false,
			Installed: false,
			Message:   fmt.Sprintf("%s %s %s", color.RedString("[X]"), message, color.RedString("not installed")),
		}, nil
	}

	out, err := exec.Command(path, "--version").CombinedOutput()
	version := strings.TrimSpace(string(out))

	if err != nil {
		return RequirementResult{}, err
	}

	if r.ExtractVersion != nil {
		version = r.ExtractVersion(version)
	}

	matched := constraint.Match(version)

	if matched {
		return RequirementResult{
			Satisfied: true,
			Installed: true,
			Message:   fmt.Sprintf("%s %s %s", color.GreenString("[✓]"), message, color.GreenString(version)),
			Version:   version,
		}, nil
	} else {
		return RequirementResult{
			Satisfied: false,
			Installed: true,
			Message:   fmt.Sprintf("%s %s %s", color.RedString("[X]"), message, color.RedString(version)),
			Version:   version,
		}, nil
	}
}
