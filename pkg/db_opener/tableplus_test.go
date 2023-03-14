package db_opener

import (
	"testing"
)

func TestUriFor(t *testing.T) {
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

	tableplus := &Tableplus{}

	actual := tableplus.uriFor(dbCredentials)

	expected := "mysql+ssh://ssh-user@ssh-host:1234/db-user:db-password@db-host/db-name?usePrivateKey=true&enviroment=wp-env"
	if actual != expected {
		t.Errorf("expected uri %s to be %s", actual, expected)
	}
}
