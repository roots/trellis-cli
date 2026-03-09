package lima

import (
	"fmt"
	"os/exec"
	"regexp"

	"github.com/mcuadros/go-version"
	"github.com/roots/trellis-cli/command"
)

const (
	VersionRequired = ">= 0.15.0"
)

func Installed() error {
	if _, err := exec.LookPath("limactl"); err != nil {
		return fmt.Errorf("Lima is not installed.")
	}

	output, err := command.Cmd("limactl", []string{"-v"}).Output()
	if err != nil {
		return fmt.Errorf("Could not determine the version of Lima.")
	}

	re := regexp.MustCompile(`([0-9]+\.[0-9]+\.[0-9]+(-alpha|-beta)?)`)
	v := re.FindStringSubmatch(string(output))

	// If no semantic version found (e.g., git hash on Linux distro packages),
	// assume it's a recent enough version since distro packages are typically up-to-date
	if len(v) < 2 {
		return nil
	}

	constraint := version.NewConstrainGroupFromString(VersionRequired)
	matched := constraint.Match(v[1])

	if !matched {
		return fmt.Errorf("Lima version %s does not satisfy required version (%s).", v[1], VersionRequired)
	}

	return nil
}
