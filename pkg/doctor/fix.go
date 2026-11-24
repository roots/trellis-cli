package doctor

import (
	"fmt"
	"slices"
	"strings"
)

// Fix is the provisioning command that brings outdated services up to date.
type Fix struct {
	Command string   `json:"command"`
	Notes   []string `json:"notes,omitempty"`
}

// extra vars that make each role upgrade its packages. Role-specific vars are
// used where they exist because they take precedence over apt_package_state
// if a project sets them in group_vars.
var upgradeExtraVars = map[Service][]string{
	ServiceNginx:   {"nginx_package_state=latest"},
	ServicePHP:     {"apt_package_state=latest"},
	ServiceMariaDB: {"mariadb_server_package_state=latest", "mariadb_client_package_state=latest"},
}

var serviceOrder = []Service{ServiceNginx, ServicePHP, ServiceMariaDB}

// NewFix returns the command that upgrades services in an environment, or
// nil if there's nothing to upgrade.
func NewFix(env string, services []Service) *Fix {
	var tags, vars []string

	for _, s := range serviceOrder {
		if !slices.Contains(services, s) {
			continue
		}
		tags = append(tags, string(s))
		vars = append(vars, upgradeExtraVars[s]...)
	}

	if len(tags) == 0 {
		return nil
	}

	fix := &Fix{
		Command: fmt.Sprintf("trellis provision --tags %s --extra-vars \"%s\" %s", strings.Join(tags, ","), strings.Join(vars, " "), env),
	}

	if slices.Contains(services, ServiceMariaDB) {
		fix.Notes = append(fix.Notes, "Back up your databases before upgrading MariaDB")
	}

	return fix
}
