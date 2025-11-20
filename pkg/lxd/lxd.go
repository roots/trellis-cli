package lxd

import (
	"fmt"
	"os/exec"
	"regexp"

	"github.com/mcuadros/go-version"
	"github.com/roots/trellis-cli/command"
)

const (
	VersionRequired = ">= 0.14.0"
)

func Installed() error {
	if _, err := exec.LookPath("lxc"); err != nil {
		return fmt.Errorf("LXD is not installed.")
	}

	output, err := command.Cmd("lxc", []string{"-v"}).Output()
	if err != nil {
		return fmt.Errorf("Could get determine the version of LXD.")
	}

	re := regexp.MustCompile(`.*([0-9]+\.[0-9]+\.[0-9]+(-alpha|beta)?)`)
	v := re.FindStringSubmatch(string(output))
	constraint := version.NewConstrainGroupFromString(VersionRequired)
	matched := constraint.Match(v[1])

	if !matched {
		return fmt.Errorf("LXD version %s does not satisfy required version (%s).", v[1], VersionRequired)
	}

	return nil
}
