package doctor

import (
	"slices"
	"testing"
)

func TestEnvFix(t *testing.T) {
	const staleNote = "The server's package lists are out of date, so this may include packages that are already current; provisioning refreshes them first"
	const backupNote = "Back up your databases before upgrading MariaDB"

	cases := []struct {
		name    string
		hosts   []HostResult
		command string
		args    []string
		notes   []string
		note    string
	}{
		{
			name:  "nothing to upgrade",
			hosts: []HostResult{{}},
		},
		{
			name:    "in command order",
			hosts:   []HostResult{{upgrades: []*Service{phpService, nginxService}}},
			command: `trellis provision --tags nginx,php --extra-vars "nginx_package_state=latest apt_package_state=latest" production`,
			args:    []string{"--tags", "nginx,php", "--extra-vars", "nginx_package_state=latest apt_package_state=latest", "production"},
		},
		{
			name:    "mariadb backup note",
			hosts:   []HostResult{{upgrades: []*Service{mariaDBService}}},
			command: `trellis provision --tags mariadb --extra-vars "mariadb_server_package_state=latest mariadb_client_package_state=latest" production`,
			notes:   []string{backupNote},
		},
		{
			name: "withheld on another host",
			hosts: []HostResult{
				{upgrades: []*Service{mariaDBService, nginxService}},
				{upgrades: []*Service{phpService}, withheld: []*Service{mariaDBService}},
			},
			command: `trellis provision --tags nginx,php --extra-vars "nginx_package_state=latest apt_package_state=latest" production`,
		},
		{
			name:  "unchecked host",
			hosts: []HostResult{{upgrades: []*Service{nginxService}}, {Error: "unreachable"}},
			note:  "not every host could be checked, and provisioning runs on all of them",
		},
		{
			name:  "unchecked host with nothing to upgrade",
			hosts: []HostResult{{}, {Error: "unreachable"}},
		},
		{
			name:    "stale package lists",
			hosts:   []HostResult{{upgrades: []*Service{nginxService, mariaDBService}, staleLists: true}},
			command: `trellis provision --tags nginx,mariadb --extra-vars "nginx_package_state=latest mariadb_server_package_state=latest mariadb_client_package_state=latest" production`,
			notes:   []string{staleNote, backupNote},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fix, note := envFix("production", tc.hosts)

			if note != tc.note {
				t.Errorf("expected note %q, got %q", tc.note, note)
			}
			if tc.command == "" {
				if fix != nil {
					t.Errorf("expected no fix, got %+v", fix)
				}
				return
			}
			if fix == nil {
				t.Fatal("expected a fix")
			}
			if fix.Command != tc.command {
				t.Errorf("got  %s\nwant %s", fix.Command, tc.command)
			}
			if tc.args != nil && !slices.Equal(fix.ProvisionArgs(), tc.args) {
				t.Errorf("ProvisionArgs() = %q, want %q", fix.ProvisionArgs(), tc.args)
			}
			if !slices.Equal(fix.Notes, tc.notes) {
				t.Errorf("expected notes %q, got %q", tc.notes, fix.Notes)
			}
		})
	}
}
