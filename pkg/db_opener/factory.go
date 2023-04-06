package db_opener

import (
	"fmt"
	"time"
)

type Factory struct{}

type Opener interface {
	Open(c DBCredentials) (err error)
}

func (f *Factory) Make(app string) (o Opener, err error) {
	switch app {
	case "tableplus":
		return &Tableplus{}, nil
	case "sequel-ace":
		return &SequelAce{spfDeleteDelay: 3 * time.Second, spfFile: nil}, nil
	case "sequel-pro":
		return nil, fmt.Errorf("Sequel Pro is replaced by Sequel Ace. Check the docs for more info: https://roots.io/trellis/docs/database-access/")
	}

	return nil, fmt.Errorf("%s is not supported", app)
}

func (f *Factory) GetSupportedApps() []string {
	return []string{
		"tableplus",
		"sequel-ace",
	}
}
