package doctor

import (
	"fmt"
	"slices"
	"strings"
)

// Fix is the provisioning command that brings outdated services up to date.
type Fix struct {
	// Command is the full command line, for display.
	Command   string   `json:"command"`
	Tags      []string `json:"tags"`
	ExtraVars []string `json:"extra_vars"`
	Notes     []string `json:"notes,omitempty"`
}

// ProvisionArgs are the `trellis provision` arguments that apply the fix.
func (f *Fix) ProvisionArgs(env string) []string {
	return []string{"--tags", strings.Join(f.Tags, ","), "--extra-vars", strings.Join(f.ExtraVars, " "), env}
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

// EnvFix returns the fix for an environment with the given hosts. Provisioning
// runs on every host in an environment, so a service is left out if any host
// withheld it, and there's no fix when a host couldn't be checked. note
// explains a missing fix when there would otherwise have been one.
func EnvFix(env string, hosts []HostReport, unchecked int) (fix *Fix, note string) {
	// confirmed are upgrades known to be needed on at least one host.
	var upgrades, withheld, confirmed []Service
	for _, h := range hosts {
		for _, s := range h.Upgrades {
			if !slices.Contains(upgrades, s) {
				upgrades = append(upgrades, s)
			}
			if !slices.Contains(h.Unconfirmed, s) {
				confirmed = append(confirmed, s)
			}
		}
		withheld = append(withheld, h.Withheld...)
	}

	upgrades = slices.DeleteFunc(upgrades, func(s Service) bool { return slices.Contains(withheld, s) })

	if len(upgrades) > 0 && unchecked > 0 {
		return nil, "not every host could be checked, and provisioning runs on all of them"
	}

	fix = NewFix(env, upgrades)
	if fix == nil {
		return nil, ""
	}

	var unconfirmed []string
	for _, s := range serviceOrder {
		if slices.Contains(upgrades, s) && !slices.Contains(confirmed, s) {
			unconfirmed = append(unconfirmed, serviceNames[s])
		}
	}

	if len(unconfirmed) > 0 {
		notes := []string{fmt.Sprintf("Includes %s because the package lists are out of date; provisioning refreshes them and only upgrades what's newer", joinAnd(unconfirmed))}
		for _, n := range fix.Notes {
			if n == mariaDBBackupNote && slices.Contains(unconfirmed, serviceNames[ServiceMariaDB]) {
				n = "If MariaDB has an update, back up your databases first"
			}
			notes = append(notes, n)
		}
		fix.Notes = notes
	}

	return fix, ""
}

var serviceNames = map[Service]string{
	ServiceNginx:   "Nginx",
	ServicePHP:     "PHP",
	ServiceMariaDB: "MariaDB",
}

const mariaDBBackupNote = "Back up your databases before upgrading MariaDB"

func joinAnd(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
	}
}

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
		Command:   fmt.Sprintf("trellis provision --tags %s --extra-vars \"%s\" %s", strings.Join(tags, ","), strings.Join(vars, " "), env),
		Tags:      tags,
		ExtraVars: vars,
	}

	if slices.Contains(services, ServiceMariaDB) {
		fix.Notes = append(fix.Notes, mariaDBBackupNote)
	}

	return fix
}
