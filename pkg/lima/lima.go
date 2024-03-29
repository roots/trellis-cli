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
		return fmt.Errorf("Could get determine the version of Lima.")
	}

	re := regexp.MustCompile(`.*([0-9]+\.[0-9]+\.[0-9]+(-alpha|beta)?)`)
	v := re.FindStringSubmatch(string(output))
	constraint := version.NewConstrainGroupFromString(VersionRequired)
	matched := constraint.Match(v[1])

	if !matched {
		return fmt.Errorf("Lima version %s does not satisfy required version (%s).", v[1], VersionRequired)
	}

	return nil
}
