package cmd

import (
	"fmt"
)

type DBOpenerFactory struct{}

type DBOpener interface {
	Open(c DBCredentials) (err error)
}

func (f *DBOpenerFactory) Make(app string) (o DBOpener, err error) {
	switch app {
	case "tableplus":
		return &DBOpenerTableplus{}, nil
	case "sequel-ace":
		return &DBOpenerSequelAce{}, nil
	case "sequel-pro":
		return nil, fmt.Errorf("Sequel Pro is replaced by Sequel Ace. Check the docs for more info: https://docs.roots.io/trellis/master/database-access/")
	}

	return nil, fmt.Errorf("%s is not supported", app)
}

func (f *DBOpenerFactory) GetSupportedApps() []string {
	return []string{
		"tableplus",
		"sequel-ace",
	}
}
