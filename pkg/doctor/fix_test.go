package doctor

import (
	"slices"
	"testing"
)

func TestNewFix(t *testing.T) {
	if fix := NewFix("production", nil); fix != nil {
		t.Errorf("expected no fix, got %+v", fix)
	}

	fix := NewFix("production", []Service{ServicePHP, ServiceNginx})
	want := `trellis provision --tags nginx,php --extra-vars "nginx_package_state=latest apt_package_state=latest" production`
	if fix.Command != want {
		t.Errorf("got  %s\nwant %s", fix.Command, want)
	}
	if len(fix.Notes) != 0 {
		t.Errorf("unexpected notes %v", fix.Notes)
	}

	wantArgs := []string{"--tags", "nginx,php", "--extra-vars", "nginx_package_state=latest apt_package_state=latest", "production"}
	if args := fix.ProvisionArgs("production"); !slices.Equal(args, wantArgs) {
		t.Errorf("ProvisionArgs() = %q, want %q", args, wantArgs)
	}

	fix = NewFix("staging", []Service{ServiceMariaDB})
	want = `trellis provision --tags mariadb --extra-vars "mariadb_server_package_state=latest mariadb_client_package_state=latest" staging`
	if fix.Command != want {
		t.Errorf("got  %s\nwant %s", fix.Command, want)
	}
	if !slices.Equal(fix.Notes, []string{"Back up your databases before upgrading MariaDB"}) {
		t.Errorf("unexpected notes %v", fix.Notes)
	}
}

func TestEnvFix(t *testing.T) {
	a := HostReport{Upgrades: []Service{ServiceMariaDB, ServiceNginx}}
	b := HostReport{Upgrades: []Service{ServicePHP}, Withheld: []Service{ServiceMariaDB}}

	fix, note := EnvFix("production", []HostReport{a, b}, 0)
	if fix == nil || note != "" {
		t.Fatalf("expected a fix, got %+v %q", fix, note)
	}
	if !slices.Equal(fix.Tags, []string{"nginx", "php"}) {
		t.Errorf("expected mariadb left out because another host withheld it, got %v", fix.Tags)
	}

	if fix, note := EnvFix("production", []HostReport{a}, 1); fix != nil || note == "" {
		t.Errorf("expected no fix with an unchecked host, got %+v %q", fix, note)
	}

	if fix, note := EnvFix("production", []HostReport{{}}, 1); fix != nil || note != "" {
		t.Errorf("expected nothing when nothing needs upgrading, got %+v %q", fix, note)
	}
}
