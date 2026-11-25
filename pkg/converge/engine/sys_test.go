package engine

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/state"
)

func TestParseSystemctlShow(t *testing.T) {
	out := "Id=nginx.service\nLoadState=loaded\nActiveState=active\nUnitFileState=enabled\n\n" +
		"Id=nope.service\nLoadState=not-found\nActiveState=inactive\nUnitFileState=\n"
	got := parseSystemctlShow([]byte(out))
	want := map[string]state.Unit{
		"nginx.service": {LoadState: "loaded", ActiveState: "active", UnitFileState: "enabled"},
		"nope.service":  {LoadState: "not-found", ActiveState: "inactive"},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error(diff)
	}
}

func TestParseDpkg(t *testing.T) {
	out := "nginx\t1.29.3-1~noble\tii \nfoo\t1.0\trc \ngit\t1:2.43\thi \n"
	want := map[string]string{"nginx": "1.29.3-1~noble", "git": "1:2.43"}
	if diff := cmp.Diff(want, parseDpkg([]byte(out))); diff != "" {
		t.Error(diff)
	}
}

func TestParseGPG(t *testing.T) {
	out := "pub:-:255:22:B188E2B695BD4743:::::::\nfpr:::::::::15058500A0235D97F5D10063B188E2B695BD4743:\nsub:...\nfpr:::::::::AAAA:\n"
	if diff := cmp.Diff([]string{"15058500A0235D97F5D10063B188E2B695BD4743", "AAAA"}, parseGPGFingerprints([]byte(out))); diff != "" {
		t.Error(diff)
	}
}

func TestSQLQuoting(t *testing.T) {
	u := &manifest.DatabaseUser{User: "o'brien", Host: "localhost", Password: `p'w\x`, Databases: []string{"we`ird"}}
	sql := userSQL(u)
	for _, want := range []string{
		`CREATE USER IF NOT EXISTS 'o\'brien'@'localhost' IDENTIFIED BY 'p\'w\\x';`,
		"GRANT ALL PRIVILEGES ON `we``ird`.* TO 'o\\'brien'@'localhost';",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("missing %q in:\n%s", want, sql)
		}
	}
	cmd := mariadbCmd(&manifest.DBServer{Host: "db", User: "admin", Password: "secret", TLS: true}, sql)
	if strings.Contains(strings.Join(cmd.Argv, " "), "secret") {
		t.Error("password must not appear in argv")
	}
	if !strings.Contains(strings.Join(cmd.Env, " "), "MYSQL_PWD=secret") {
		t.Error("password should be passed via env")
	}
}

func TestAptInstallPins(t *testing.T) {
	cmd := aptInstallCmd([]manifest.Package{{Name: "nginx", Version: "1.2"}, {Name: "git"}})
	got := strings.Join(cmd.Argv, " ")
	if !strings.HasSuffix(got, "-- nginx=1.2 git") {
		t.Errorf("argv = %s", got)
	}
}
