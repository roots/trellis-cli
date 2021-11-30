package cmd

import (
	"github.com/mitchellh/cli"
	"regexp"
	"strings"
	"testing"
)

func TestOpen(t *testing.T) {
	defer MockExec(t)()

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
	sequelAce := &DBOpenerSequelAce{
		ui: ui,
	}

	sequelAce.Open(dbCredentials)

	actualCombined := ui.OutputWriter.String() + ui.ErrorWriter.String()
	actualCombined = strings.TrimSpace(actualCombined)

	pattern := `open .*\.spf`

	matched, _ := regexp.MatchString(pattern, actualCombined)

	if !matched {
		t.Errorf("expected command %q to match %q", actualCombined, pattern)
	}
}
