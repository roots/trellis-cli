package doctor

import (
	"errors"
	"strings"
	"testing"
)

func TestParsePlaybookOutput(t *testing.T) {
	runs, err := parsePlaybookOutput([]byte(readFixture(t, "playbook_ok.json")))
	if err != nil {
		t.Fatal(err)
	}

	if len(runs) != 1 {
		t.Fatalf("expected 1 host, got %d", len(runs))
	}

	run := runs[0]
	if run.Name != "default" || run.Address != "127.0.0.1" || run.User != "admin" || run.Error != "" {
		t.Errorf("unexpected run: %+v", run)
	}

	want := projectVars{PHPVersion: "8.4", MariaDBVersion: "10.11", NginxPackage: "nginx", MariaDBServerPackage: "mariadb-server"}
	if run.Vars != want {
		t.Errorf("unexpected vars: %+v", run.Vars)
	}

	facts, err := parseProbe(run.Probe)
	if err != nil {
		t.Fatalf("probe output didn't parse: %v", err)
	}
	if facts.Packages["nginx"] == nil {
		t.Error("expected nginx in probe output")
	}
}

func TestParsePlaybookOutputUnreachable(t *testing.T) {
	runs, err := parsePlaybookOutput([]byte(readFixture(t, "playbook_unreachable.json")))
	if err != nil {
		t.Fatal(err)
	}

	if len(runs) != 1 {
		t.Fatalf("expected 1 host, got %d", len(runs))
	}

	run := runs[0]
	if run.Name != "bogus" || run.Address != "192.0.2.10" || run.Probe != "" {
		t.Errorf("unexpected run: %+v", run)
	}
	if !strings.Contains(run.Error, "Failed to connect to the host via ssh") {
		t.Errorf("unexpected error %q", run.Error)
	}
}

func TestParsePlaybookOutputNoHosts(t *testing.T) {
	_, err := parsePlaybookOutput([]byte(readFixture(t, "playbook_no_hosts.json")))
	if !errors.Is(err, ErrNoHosts) {
		t.Errorf("expected ErrNoHosts, got %v", err)
	}
}

func TestParsePlaybookOutputFailedTask(t *testing.T) {
	out := `{
  "plays": [{"tasks": [
    {"task": {"name": "connection : Warn about change in host keys"}, "hosts": {"web1": {"failed": true, "msg": "WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!"}}},
    {"task": {"name": "Trellis CLI: Doctor vars"}, "hosts": {"web2": {"msg": {"host": "web2", "user": "admin", "vars": {}}}}},
    {"task": {"name": "Trellis CLI: Doctor probe"}, "hosts": {"web2": {"rc": 0, "stdout": "doctor_begin v=1\r\ndoctor_end\r\n"}}}
  ]}],
  "stats": {"web1": {"failures": 1}, "web2": {"ok": 1}}
}`

	runs, err := parsePlaybookOutput([]byte(out))
	if err != nil {
		t.Fatal(err)
	}

	if len(runs) != 2 || runs[0].Name != "web1" || runs[1].Name != "web2" {
		t.Fatalf("unexpected runs: %+v", runs)
	}
	if !strings.Contains(runs[0].Error, "REMOTE HOST IDENTIFICATION HAS CHANGED") {
		t.Errorf("unexpected web1 error %q", runs[0].Error)
	}
	if runs[1].Error != "" || runs[1].Probe == "" {
		t.Errorf("unexpected web2 run: %+v", runs[1])
	}
}

func TestParsePlaybookOutputInvalid(t *testing.T) {
	if _, err := parsePlaybookOutput([]byte("ERROR! Attempting to decrypt but no vault secrets found")); err == nil {
		t.Error("expected an error for non-JSON output")
	}
}

func TestParsePlaybookOutputLongError(t *testing.T) {
	msg := "WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!" + strings.Repeat("\\nverbose ssh output", 200)
	out := `{"plays": [{"tasks": [{"task": {"name": "connection : Warn about change in host keys"}, "hosts": {"web1": {"failed": true, "msg": "` + msg + `"}}}]}], "stats": {"web1": {"failures": 1}}}`

	runs, err := parsePlaybookOutput([]byte(out))
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(runs[0].Error, "\n")
	if len(lines) != 9 || !strings.HasPrefix(lines[0], "WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!") || lines[8] != "…" {
		t.Errorf("expected the error capped at 8 lines, got %d: %q", len(lines), runs[0].Error)
	}
}

func TestParsePlaybookOutputMissingVars(t *testing.T) {
	// The vars message doesn't decode (a version arrived as a number), so the
	// host can't be evaluated reliably.
	out := `{
  "plays": [{"tasks": [
    {"task": {"name": "Trellis CLI: Doctor vars"}, "hosts": {"web1": {"msg": {"host": "203.0.113.10", "user": "admin", "vars": {"php_version": 8.3}}}}},
    {"task": {"name": "Trellis CLI: Doctor probe"}, "hosts": {"web1": {"rc": 0, "stdout": "doctor_begin v=1\ndoctor_end\n"}, "web2": {"rc": 0, "stdout": "doctor_begin v=1\ndoctor_end\n"}}}
  ]}],
  "stats": {"web1": {"ok": 2}, "web2": {"ok": 1}}
}`

	runs, err := parsePlaybookOutput([]byte(out))
	if err != nil {
		t.Fatal(err)
	}

	for _, run := range runs {
		if !strings.Contains(run.Error, "couldn't read the project's variables") {
			t.Errorf("%s: expected a vars error, got %q", run.Name, run.Error)
		}
	}
}
