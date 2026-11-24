package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/doctor"
	"github.com/roots/trellis-cli/trellis"
)

// mockAnsiblePlaybook makes every command print output (as ansible-playbook
// with the json callback would) and records each command's args.
func mockAnsiblePlaybook(t *testing.T, output string) *[][]string {
	t.Helper()

	var mu sync.Mutex
	calls := &[][]string{}

	command.Mock(func(name string, args []string) *exec.Cmd {
		mu.Lock()
		*calls = append(*calls, append([]string{name}, args...))
		mu.Unlock()

		cmd := exec.Command(os.Args[0], "-test.run=TestDoctorHelperProcess", "--")
		cmd.Env = []string{"GO_WANT_DOCTOR_HELPER_PROCESS=1", "DOCTOR_HELPER_OUTPUT=" + output}
		return cmd
	})
	t.Cleanup(command.Restore)

	return calls
}

func TestDoctorHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_DOCTOR_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprint(os.Stdout, os.Getenv("DOCTOR_HELPER_OUTPUT"))
	os.Exit(0)
}

func readDoctorFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../pkg/doctor/testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDoctorArgumentValidations(t *testing.T) {
	cases := []struct {
		name            string
		projectDetected bool
		args            []string
		out             string
		code            int
	}{
		{"no_project", false, nil, "No Trellis project detected", 2},
		{"invalid_env", true, []string{"--offline", "nope"}, "Error: nope is not a valid environment", 2},
		{"option_after_env", true, []string{"production", "--json"}, "Options must come before environment names", 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.projectDetected {
				defer trellis.LoadFixtureProject(t)()
			}

			ui := cli.NewMockUi()
			c := NewDoctorCommand(ui, trellis.NewMockTrellis(tc.projectDetected), "1.0.0")
			if tc.projectDetected {
				c = NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0")
			}

			code := c.Run(tc.args)

			if code != tc.code {
				t.Errorf("expected code %d, got %d", tc.code, code)
			}
			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}
		})
	}
}

func TestDoctorRunEnvironment(t *testing.T) {
	output := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, output)

	ui := cli.NewMockUi()
	tp := trellis.NewTrellis()
	code := NewDoctorCommand(ui, tp, "1.0.0").Run([]string{"--offline", "production"})

	text := ui.OutputWriter.String()

	if code != 1 {
		t.Errorf("expected exit code 1 for outdated packages, got %d\n%s%s", code, text, ui.ErrorWriter.String())
	}

	for _, want := range []string{
		"production · admin@default (127.0.0.1)",
		"✗ nginx",
		"Fix:",
		"trellis provision --tags nginx,php,mariadb",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in:\n%s", want, text)
		}
	}

	if len(*calls) != 1 {
		t.Fatalf("expected 1 ansible-playbook run, got %v", *calls)
	}
	call := (*calls)[0]
	if call[0] != "ansible-playbook" || !strings.HasPrefix(call[1], doctor.FilePrefix) || !slices.Contains(call, "-e env=production") {
		t.Errorf("unexpected command %v", call)
	}
	if !slices.ContainsFunc(call, func(arg string) bool { return strings.HasPrefix(arg, "-e "+doctor.ProbeVar+"="+doctor.FilePrefix) }) {
		t.Errorf("expected the probe file as an extra var, got %v", call)
	}

	if leftovers, _ := filepath.Glob(doctor.FilePrefix + "*"); len(leftovers) > 0 {
		t.Errorf("expected temporary files to be removed, found %v", leftovers)
	}
}

func TestDoctorRunJSON(t *testing.T) {
	output := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	mockAnsiblePlaybook(t, output)

	ui := cli.NewMockUi()
	NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0").Run([]string{"--offline", "--json", "production"})

	var report doctor.Report
	if err := json.Unmarshal(ui.OutputWriter.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, ui.OutputWriter.String())
	}

	if len(report.Environments) != 1 || report.Environments[0].Fix == nil {
		t.Errorf("unexpected report %+v", report.Environments)
	}
}

func TestDoctorRunImplicitEnvironments(t *testing.T) {
	output := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, output)

	ui := cli.NewMockUi()
	NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0").Run([]string{"--offline"})

	text := ui.OutputWriter.String()

	// development isn't checked unless asked for.
	var envs []string
	for _, call := range *calls {
		for _, arg := range call {
			if env, ok := strings.CutPrefix(arg, "-e env="); ok {
				envs = append(envs, env)
			}
		}
	}
	slices.Sort(envs)
	if !slices.Equal(envs, []string{"production", "valet-link"}) {
		t.Errorf("expected production and valet-link to run, got %v", envs)
	}
	if strings.Contains(text, "development") {
		t.Errorf("didn't expect development:\n%s", text)
	}
}

func TestDoctorRunExplicitPlaceholderEnvironment(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, "")

	placeholder := "[production]\nyour_server_hostname\n\n[web]\nyour_server_hostname\n"
	if err := os.WriteFile("hosts/production", []byte(placeholder), 0644); err != nil {
		t.Fatal(err)
	}

	ui := cli.NewMockUi()
	code := NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0").Run([]string{"--offline", "production"})

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if len(*calls) != 0 {
		t.Errorf("expected no ansible-playbook run, got %v", *calls)
	}
	if text := ui.OutputWriter.String(); !strings.Contains(text, "? couldn't check: hosts/production still has the placeholder host") {
		t.Errorf("expected placeholder error:\n%s", text)
	}
}

func TestDoctorRunUnreachable(t *testing.T) {
	output := readDoctorFixture(t, "playbook_unreachable.json")
	defer trellis.LoadFixtureProject(t)()
	mockAnsiblePlaybook(t, output)

	ui := cli.NewMockUi()
	code := NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0").Run([]string{"--offline", "production"})

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if text := ui.OutputWriter.String(); !strings.Contains(text, "Failed to connect to the host via ssh") {
		t.Errorf("expected connection error:\n%s", text)
	}
}

func TestDoctorRunNoHosts(t *testing.T) {
	output := readDoctorFixture(t, "playbook_no_hosts.json")
	defer trellis.LoadFixtureProject(t)()
	mockAnsiblePlaybook(t, output)

	ui := cli.NewMockUi()
	code := NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0").Run([]string{"--offline", "production"})

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if text := ui.OutputWriter.String(); !strings.Contains(text, "no hosts in both the [production] and [web] groups") {
		t.Errorf("expected no hosts error:\n%s", text)
	}
}

func TestDoctorRunAnsibleFailure(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	mockAnsiblePlaybook(t, "ERROR! Attempting to decrypt but no vault secrets found")

	ui := cli.NewMockUi()
	code := NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0").Run([]string{"--offline", "production"})

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if text := ui.OutputWriter.String(); !strings.Contains(text, "couldn't parse ansible-playbook output") {
		t.Errorf("expected parse error:\n%s", text)
	}
}

func TestDoctorRunInventoryLayouts(t *testing.T) {
	output := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, output)

	// An inventory directory instead of a hosts file is left to Ansible.
	if err := os.Remove("hosts/production"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("hosts/production", 0755); err != nil {
		t.Fatal(err)
	}

	ui := cli.NewMockUi()
	NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0").Run([]string{"--offline", "production"})

	if len(*calls) != 1 {
		t.Errorf("expected ansible-playbook to run, got %v\n%s", *calls, ui.OutputWriter.String())
	}
}

func TestDoctorRunJSONCallbackMissing(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	command.Mock(func(name string, args []string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], "-test.run=TestDoctorStderrHelperProcess", "--")
		cmd.Env = []string{"GO_WANT_DOCTOR_STDERR_HELPER=1"}
		return cmd
	})
	defer command.Restore()

	ui := cli.NewMockUi()
	code := NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0").Run([]string{"--offline", "production"})

	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if text := ui.OutputWriter.String(); !strings.Contains(text, "ansible-galaxy collection install ansible.posix") {
		t.Errorf("expected ansible.posix hint:\n%s", text)
	}
}

func TestDoctorStderrHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_DOCTOR_STDERR_HELPER") != "1" {
		return
	}

	fmt.Fprint(os.Stderr, "[ERROR]: Could not load 'json' callback plugin")
	os.Exit(1)
}

func TestIsTruthy(t *testing.T) {
	for _, s := range []string{"1", "true", "True", "yes", "on"} {
		if !isTruthy(s) {
			t.Errorf("expected %q to be truthy", s)
		}
	}
	for _, s := range []string{"", "0", "false", "no"} {
		if isTruthy(s) {
			t.Errorf("expected %q not to be truthy", s)
		}
	}
}
