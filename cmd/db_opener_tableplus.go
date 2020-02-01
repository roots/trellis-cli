package cmd

import (
	"fmt"
)

type DBOpenerTableplus struct{}

func (o *DBOpenerTableplus) open(c DBCredentials) (err error) {
	uri := o.uriFor(c)
	open := execCommand("open", uri)

	// Intentionally omitting `logCmd` to prevent printing db credentials.
	if err := open.Run(); err != nil {
		return fmt.Errorf("Error opening database with Tableplus: %s", err)
	}

	return nil
}

func (o *DBOpenerTableplus) uriFor(c DBCredentials) string {
	return fmt.Sprintf(
		"mysql+ssh://%s@%s:%d/%s:%s@%s/%s?usePrivateKey=true&enviroment=%s",
		c.SSHUser,
		c.SSHHost,
		c.SSHPort,
		c.DBUser,
		c.DBPassword,
		c.DBHost,
		c.DBName,
		c.WPEnv,
	)
}
