package doctor

import (
	"bytes"
	"strings"
	"testing"
)

func testReport() *Report {
	return &Report{
		Local: []Result{
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
				Fix: newFix("production", []*Service{phpService}),
			},
			{Name: "staging", Status: EnvSkipped, Reason: "hosts/staging still has the placeholder host your_server_hostname"},
		},
	}
}

func TestWriteText(t *testing.T) {
	checked := EnvReport{Name: "production", Status: EnvChecked, Hosts: []HostResult{{Name: "web1", Results: []Result{{ID: "nginx", Title: "nginx", Status: StatusOK}}}}}

	cases := []struct {
		name     string
		report   *Report
		quiet    bool
		want     []string
		unwanted []string
	}{
		{
			name:   "full",
			report: testReport(),
			want: []string{
				"Local\n",
				"! Trellis  v1.31.0 → v1.33.0 available",
				"production · admin@example.com (203.0.113.10)",
				"✓ nginx       1.31.9",
				"✗ php8.3-fpm  8.3.36 → 8.3.37 available",
				"Fix:",
				`trellis provision --tags php --extra-vars "apt_package_state=latest" production`,
				"staging · skipped: hosts/staging still has the placeholder host",
			},
		},
		{
			name:     "quiet only shows problems",
			report:   testReport(),
			quiet:    true,
			want:     []string{"Trellis", "php8.3-fpm", "Fix:"},
			unwanted: []string{"nginx", "staging"},
		},
		{
			name:   "quiet and healthy prints nothing",
			report: &Report{Environments: []EnvReport{checked, {Name: "staging", Status: EnvSkipped, Reason: "placeholder"}}},
			quiet:  true,
		},
		{
			name: "couldn't check",
			report: &Report{Environments: []EnvReport{
				{Name: "staging", Status: EnvError, Reason: "ansible-playbook failed:\nERROR! line two"},
				{Name: "production", Status: EnvChecked, Hosts: []HostResult{
					{Name: "web1", Error: "Failed to connect to the host via ssh"},
					{Name: "web2", Results: []Result{{ID: "nginx", Title: "nginx", Status: StatusOK}}},
				}},
			}},
			quiet: true,
			want: []string{
				"staging\n  ? couldn't check: ansible-playbook failed:\n    ERROR! line two",
				"production · web1\n  ? couldn't check: Failed to connect",
			},
			unwanted: []string{"web2"},
		},
		{
			name: "fix withheld",
			report: &Report{Environments: []EnvReport{{
				Name: "production", Status: EnvChecked, FixNote: "not every host could be checked, and provisioning runs on all of them",
				Hosts: []HostResult{{Name: "web1", Error: "unreachable"}},
			}}},
			quiet: true,
			want:  []string{"Fix: No upgrade command suggested because not every host could be checked, and provisioning runs on all of them."},
		},
		{
			name:   "nothing checked",
			report: &Report{Environments: []EnvReport{{Name: "staging", Status: EnvSkipped, Reason: "placeholder"}}},
			quiet:  true,
			want:   []string{"No servers were checked. Name an environment"},
		},
		{
			name:     "nothing checked, environments named",
			report:   &Report{Environments: []EnvReport{{Name: "staging", Status: EnvError, Reason: "placeholder"}}},
			quiet:    true,
			want:     []string{"No servers were checked."},
			unwanted: []string{"Name an environment"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			tc.report.WriteText(&out, tc.quiet)
			text := out.String()

			if tc.want == nil && text != "" {
				t.Errorf("expected no output, got %q", text)
			}
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("expected %q in:\n%s", want, text)
				}
			}
			for _, unwanted := range tc.unwanted {
				if strings.Contains(text, unwanted) {
					t.Errorf("didn't expect %q in:\n%s", unwanted, text)
				}
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	checked := EnvReport{Name: "production", Status: EnvChecked, Hosts: []HostResult{{Name: "web1"}}}

	cases := map[string]struct {
		report *Report
		want   int
	}{
		"warnings":         {&Report{Local: []Result{{Status: StatusWarn}}, Environments: []EnvReport{checked}}, 0},
		"failures":         {testReport(), 1},
		"skipped env":      {&Report{Environments: []EnvReport{checked, {Name: "staging", Status: EnvSkipped}}}, 0},
		"unreachable host": {&Report{Environments: []EnvReport{{Name: "production", Status: EnvChecked, Hosts: []HostResult{{Name: "web1", Error: "unreachable"}}}}}, 2},
		"env error":        {&Report{Environments: []EnvReport{checked, {Name: "staging", Status: EnvError}}}, 2},
		"nothing checked":  {&Report{Environments: []EnvReport{{Name: "staging", Status: EnvSkipped}}}, 2},
		"unknown result":   {&Report{Local: []Result{{Status: StatusUnknown}}, Environments: []EnvReport{checked}}, 2},
		"failure and unknown": {&Report{
			Local:        []Result{{Status: StatusUnknown}, {Status: StatusFail}},
			Environments: []EnvReport{checked},
		}, 1},
	}

	for name, tc := range cases {
		if got := tc.report.ExitCode(); got != tc.want {
			t.Errorf("%s: expected %d, got %d", name, tc.want, got)
		}
	}
}
