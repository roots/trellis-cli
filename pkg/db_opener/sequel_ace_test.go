package db_opener

import (
	"os"
	"testing"
	"time"

	"github.com/roots/trellis-cli/command"
)

func TestOpen(t *testing.T) {
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

	dir := t.TempDir()
	path := dir + "/sequel-ace.spf"
	if err := os.WriteFile(path, []byte(`foo`), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	spfFile, _ := os.OpenFile(path, os.O_WRONLY, os.ModePerm)

	commands := []command.MockCommand{
		{
			Command:  "open",
			Args:     []string{path},
			Output:   "foo",
			ExitCode: 0,
		},
	}

	defer command.MockExecCommands(t, commands)()

	sequelAce := &SequelAce{spfDeleteDelay: 0 * time.Second, spfFile: spfFile}
	err := sequelAce.Open(dbCredentials)

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
}

func TestCommandHelperProcess(t *testing.T) {
	command.CommandHelperProcess(t)
}
