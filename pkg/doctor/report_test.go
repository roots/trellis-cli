package doctor

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func testReport() *Report {
	return &Report{
		Schema: ReportSchema,
		Local: []Result{
			{ID: "requirement-python", Title: "Python", Status: StatusOK, Message: "3.12.4"},
			{ID: "trellis-version", Title: "Trellis", Status: StatusWarn, Message: "v1.31.0 → v1.33.0 available", Details: []string{"https://github.com/roots/trellis/compare/v1.31.0...v1.33.0"}},
		},
		Environments: []EnvReport{
			{
				Name:   "production",
				Status: EnvChecked,
				Hosts: []HostResult{{
					Name:    "example.com",
					Address: "203.0.113.10",
					User:    "admin",
					Results: []Result{
						{ID: "nginx", Title: "nginx", Status: StatusOK, Message: "1.31.9"},
						{ID: "php", Title: "php8.3-fpm", Status: StatusFail, Message: "8.3.36 → 8.3.37 available"},
					},
				}},
				Fix: NewFix("production", []Service{ServicePHP}),
			},
			{Name: "staging", Status: EnvSkipped, Reason: "hosts/staging still has the placeholder host your_server_hostname"},
		},
	}
}

func TestWriteText(t *testing.T) {
	var out bytes.Buffer
	testReport().WriteText(&out, false)
	text := out.String()

	for _, want := range []string{
		"Local\n",
		"✓ Python   3.12.4",
		"! Trellis  v1.31.0 → v1.33.0 available",
		"production · admin@example.com (203.0.113.10)",
		"✗ php8.3-fpm  8.3.36 → 8.3.37 available",
		"Fix:",
		`trellis provision --tags php --extra-vars "apt_package_state=latest" production`,
		"staging · skipped: hosts/staging still has the placeholder host",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in:\n%s", want, text)
		}
	}
}

func TestWriteTextQuiet(t *testing.T) {
	var out bytes.Buffer
	testReport().WriteText(&out, true)
	text := out.String()

	for _, unwanted := range []string{"Python", "nginx", "staging"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("didn't expect %q in quiet output:\n%s", unwanted, text)
		}
	}
	for _, want := range []string{"Trellis", "php8.3-fpm", "Fix:"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in quiet output:\n%s", want, text)
		}
	}

	healthy := &Report{
		Local: []Result{{ID: "requirement-python", Title: "Python", Status: StatusOK}},
		Environments: []EnvReport{
			{Name: "production", Status: EnvChecked, Hosts: []HostResult{{Name: "web1", Results: []Result{{ID: "nginx", Title: "nginx", Status: StatusOK}}}}},
			{Name: "staging", Status: EnvSkipped, Reason: "placeholder"},
		},
	}
	out.Reset()
	healthy.WriteText(&out, true)
	if out.Len() != 0 {
		t.Errorf("expected no quiet output when healthy, got %q", out.String())
	}
}

func TestWriteTextErrors(t *testing.T) {
	report := &Report{Environments: []EnvReport{
		{Name: "staging", Status: EnvError, Reason: "ansible-playbook failed"},
		{Name: "production", Status: EnvChecked, Hosts: []HostResult{
			{Name: "web1", Error: "Failed to connect to the host via ssh"},
			{Name: "web2", Results: []Result{{ID: "nginx", Title: "nginx", Status: StatusOK, Message: "1.31.9"}}},
		}},
	}}

	var out bytes.Buffer
	report.WriteText(&out, true)
	text := out.String()

	for _, want := range []string{"staging\n  ? couldn't check: ansible-playbook failed", "production · web1\n  ? couldn't check: Failed to connect"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "web2") {
		t.Errorf("didn't expect healthy host in quiet output:\n%s", text)
	}
}

func TestExitCode(t *testing.T) {
	warnOnly := &Report{Local: []Result{{Status: StatusWarn}}, Environments: []EnvReport{{Name: "production", Status: EnvChecked}}}
	if got := warnOnly.ExitCode(false); got != 0 {
		t.Errorf("warnings: expected 0, got %d", got)
	}
	if got := warnOnly.ExitCode(true); got != 1 {
		t.Errorf("strict warnings: expected 1, got %d", got)
	}

	if got := testReport().ExitCode(false); got != 1 {
		t.Errorf("failures: expected 1, got %d", got)
	}

	unreachable := &Report{Environments: []EnvReport{{Name: "production", Status: EnvChecked, Hosts: []HostResult{{Name: "web1", Error: "unreachable"}}}}}
	if got := unreachable.ExitCode(false); got != 2 {
		t.Errorf("unreachable host: expected 2, got %d", got)
	}

	envError := &Report{Environments: []EnvReport{{Name: "production", Status: EnvError}}}
	if got := envError.ExitCode(false); got != 2 {
		t.Errorf("environment error: expected 2, got %d", got)
	}

	checked := EnvReport{Name: "production", Status: EnvChecked, Hosts: []HostResult{{Name: "web1"}}}

	skipped := &Report{Environments: []EnvReport{checked, {Name: "staging", Status: EnvSkipped}}}
	if got := skipped.ExitCode(false); got != 0 {
		t.Errorf("skipped environment: expected 0, got %d", got)
	}

	nothingChecked := &Report{Environments: []EnvReport{{Name: "staging", Status: EnvSkipped}}}
	if got := nothingChecked.ExitCode(false); got != 2 {
		t.Errorf("nothing checked: expected 2, got %d", got)
	}

	unknown := &Report{Local: []Result{{Status: StatusUnknown}}, Environments: []EnvReport{checked}}
	if got := unknown.ExitCode(true); got != 2 {
		t.Errorf("unknown result: expected 2, got %d", got)
	}
}

func TestWriteTextNothingChecked(t *testing.T) {
	report := &Report{Environments: []EnvReport{{Name: "staging", Status: EnvSkipped, Reason: "placeholder"}}}

	var out bytes.Buffer
	report.WriteText(&out, true)

	if !strings.Contains(out.String(), "No servers were checked. Name an environment") {
		t.Errorf("expected a notice in quiet output, got %q", out.String())
	}

	named := &Report{Environments: []EnvReport{{Name: "staging", Status: EnvError, Reason: "placeholder"}}}
	out.Reset()
	named.WriteText(&out, true)

	if text := out.String(); !strings.Contains(text, "No servers were checked.") || strings.Contains(text, "Name an environment") {
		t.Errorf("expected a notice without the suggestion, got %q", text)
	}
}

func TestWriteTextIndentsMultilineErrors(t *testing.T) {
	report := &Report{Environments: []EnvReport{{Name: "production", Status: EnvError, Reason: "ansible-playbook failed:\nERROR! line two"}}}

	var out bytes.Buffer
	report.WriteText(&out, false)

	if !strings.Contains(out.String(), "\n    ERROR! line two") {
		t.Errorf("expected indented continuation, got:\n%s", out.String())
	}
}

func TestWriteJSON(t *testing.T) {
	var out bytes.Buffer
	if err := testReport().WriteJSON(&out); err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out.String())
	}

	if decoded["schema"] != float64(ReportSchema) {
		t.Errorf("unexpected schema %v", decoded["schema"])
	}

	envs := decoded["environments"].([]any)
	prod := envs[0].(map[string]any)
	if prod["fix"].(map[string]any)["command"] == "" {
		t.Error("expected a fix command")
	}
	if staging := envs[1].(map[string]any); staging["status"] != "skipped" || staging["reason"] == "" {
		t.Errorf("unexpected staging %v", staging)
	}
}

func TestWriteTextFixNote(t *testing.T) {
	report := &Report{Environments: []EnvReport{{
		Name: "production", Status: EnvChecked, FixNote: "not every host could be checked, and provisioning runs on all of them",
		Hosts: []HostResult{{Name: "web1", Error: "unreachable"}},
	}}}

	var out bytes.Buffer
	report.WriteText(&out, true)

	if want := "Fix: No upgrade command suggested because not every host could be checked, and provisioning runs on all of them."; !strings.Contains(out.String(), want) {
		t.Errorf("expected %q in:\n%s", want, out.String())
	}
}
