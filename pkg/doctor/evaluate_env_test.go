package doctor

import (
	"errors"
	"strings"
	"testing"
)

func TestEvaluateEnv(t *testing.T) {
	report, err := EvaluateEnv("development", []byte(readFixture(t, "playbook_ok.json")), true)
	if err != nil {
		t.Fatal(err)
	}

	if report.Status != EnvChecked || len(report.Hosts) != 1 {
		t.Fatalf("unexpected report %+v", report)
	}
	host := report.Hosts[0]
	if host.Name != "default" || host.Address != "127.0.0.1" || host.User != "admin" || len(host.Results) == 0 {
		t.Errorf("unexpected host %+v", host)
	}
	if report.Fix == nil || !strings.Contains(report.Fix.Command, "--tags nginx,php,mariadb") || !strings.HasSuffix(report.Fix.Command, " development") {
		t.Errorf("unexpected fix %+v", report.Fix)
	}
}

func TestEvaluateEnvHostErrors(t *testing.T) {
	out := `{
  "plays": [{"tasks": [
    {"task": {"name": "Trellis CLI: Doctor vars"}, "hosts": {"web2": {"msg": {"host": "web2", "user": "admin", "vars": {}}}}},
    {"task": {"name": "Trellis CLI: Doctor probe"}, "hosts": {
      "web1": {"unreachable": true, "msg": "Failed to connect to the host via ssh: Host key verification failed."},
      "web2": {"rc": 0, "stdout": "doctor_begin v=1\nnow=1\n"}
    }}
  ]}],
  "stats": {"web1": {"unreachable": 1}, "web2": {"ok": 1}}
}`

	report, err := EvaluateEnv("production", []byte(out), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Hosts) != 2 {
		t.Fatalf("expected 2 hosts, got %+v", report.Hosts)
	}

	if !strings.Contains(report.Hosts[0].Error, "trellis ssh production") {
		t.Errorf("expected a host key hint, got %q", report.Hosts[0].Error)
	}
	if !strings.Contains(report.Hosts[1].Error, "truncated") {
		t.Errorf("expected a truncated probe error, got %q", report.Hosts[1].Error)
	}
}

func TestEvaluateEnvNoHosts(t *testing.T) {
	if _, err := EvaluateEnv("production", []byte(readFixture(t, "playbook_no_hosts.json")), true); !errors.Is(err, ErrNoHosts) {
		t.Errorf("expected ErrNoHosts, got %v", err)
	}
}
