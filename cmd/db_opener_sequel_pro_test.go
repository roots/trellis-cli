package cmd

import (
	"strings"
	"testing"

	"os/exec"

	"github.com/mitchellh/cli"
)

func TestOpen(t *testing.T) {
	execCommand = mockExecCommand
	defer func() { execCommand = exec.Command }()

	dbCredentials := DBCredentials{
		SSHUser:    "ssh-user",
		SSHHost:    "ssh-host",
		SSHPort:    1234,
		DBUser:     "db-user",
		DBPassword: "db-password",
		DBHost:     "db-host",
		DBName:     "db-name",
		WPEnv:      "wp-env",
	}

	ui := cli.NewMockUi()
	sequelPro := &DBOpenerSequelPro{
		ui: ui,
	}

	sequelPro.open(dbCredentials)

	actualCombined := ui.OutputWriter.String() + ui.ErrorWriter.String()
	actualCombined = strings.TrimSpace(actualCombined)

	expectedPrefix := "open"
	if !strings.HasPrefix(actualCombined, expectedPrefix) {
		t.Errorf("expected command %q to have prefix %q", actualCombined, expectedPrefix)
	}

	expectedSuffix := ".spf"
	if !strings.HasSuffix(actualCombined, expectedSuffix) {
		t.Errorf("expected command %q to have suffix %q", actualCombined, expectedSuffix)
	}
}
