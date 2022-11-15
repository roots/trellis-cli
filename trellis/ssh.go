package trellis

import (
	"fmt"
)

func (t *Trellis) SshHost(environment string, siteName string, user string) string {
	host := t.SiteFromEnvironmentAndName(environment, siteName).MainHost()

	if environment == "development" {
		user = "vagrant"
	} else {
		if user == "" {
			user = "admin"
		}
	}

	return fmt.Sprintf("%s@%s", user, host)
}
