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

	fix = NewFix("staging", []Service{ServiceMariaDB})
	want = `trellis provision --tags mariadb --extra-vars "mariadb_server_package_state=latest mariadb_client_package_state=latest" staging`
	if fix.Command != want {
		t.Errorf("got  %s\nwant %s", fix.Command, want)
	}
	if !slices.Equal(fix.Notes, []string{"Back up your databases before upgrading MariaDB"}) {
		t.Errorf("unexpected notes %v", fix.Notes)
	}
}
