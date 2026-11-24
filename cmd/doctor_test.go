package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/doctor"
	"github.com/roots/trellis-cli/trellis"
)

// mockAnsiblePlaybook makes doctor's playbook runs print output (as
// ansible-playbook with the json callback would), makes every other command
// succeed silently, and records each command's args.
func mockAnsiblePlaybook(t *testing.T, output string) *[][]string {
	t.Helper()
	return mockAnsible(t, []string{output}, 0)
}

// mockAnsible is mockAnsiblePlaybook with a different output for each doctor
// playbook run (the last one repeats) and an exit code for every other
// command, such as provisioning.
func mockAnsible(t *testing.T, outputs []string, otherExitCode int) *[][]string {
	t.Helper()

	var mu sync.Mutex
	calls := &[][]string{}
	doctorRuns := 0

	command.Mock(func(name string, args []string) *exec.Cmd {
		mu.Lock()
		defer mu.Unlock()
		*calls = append(*calls, append([]string{name}, args...))

		commandOutput, exitCode := "", otherExitCode
		if len(args) > 0 && strings.HasPrefix(args[0], doctor.FilePrefix) {
			commandOutput = outputs[min(doctorRuns, len(outputs)-1)]
			exitCode = 0
			doctorRuns++
		}

		cmd := exec.Command(os.Args[0], "-test.run=TestDoctorHelperProcess", "--")
		cmd.Env = []string{
			"GO_WANT_DOCTOR_HELPER_PROCESS=1",
			"DOCTOR_HELPER_OUTPUT=" + commandOutput,
			fmt.Sprintf("DOCTOR_HELPER_EXIT_CODE=%d", exitCode),
		}
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
	code, _ := strconv.Atoi(os.Getenv("DOCTOR_HELPER_EXIT_CODE"))
	os.Exit(code)
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

func newFixCommand(ui *cli.MockUi, answers string) *DoctorCommand {
	ui.InputReader = strings.NewReader(answers)
	c := NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0")
	c.stdinIsTerminal = func() bool { return true }
	return c
}

func playbookCalls(calls [][]string, playbook string) [][]string {
	var matching [][]string
	for _, call := range calls {
		if len(call) > 1 && strings.HasPrefix(call[1], playbook) {
			matching = append(matching, call)
		}
	}
	return matching
}

func TestDoctorFix(t *testing.T) {
	output := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, output)

	ui := cli.NewMockUi()
	code := newFixCommand(ui, "y\n").Run([]string{"--offline", "--fix", "production"})

	// The mocked servers are still outdated after "provisioning".
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}

	provisions := playbookCalls(*calls, "server.yml")
	if len(provisions) != 1 {
		t.Fatalf("expected 1 provision run, got %v", *calls)
	}
	want := []string{"ansible-playbook", "server.yml", "-e nginx_package_state=latest apt_package_state=latest mariadb_server_package_state=latest mariadb_client_package_state=latest", "--tags=nginx,php,mariadb", "-e env=production"}
	if !slices.Equal(provisions[0], want) {
		t.Errorf("unexpected provision command\ngot  %q\nwant %q", provisions[0], want)
	}

	if checks := playbookCalls(*calls, doctor.FilePrefix); len(checks) != 2 {
		t.Errorf("expected doctor to check production twice, got %d runs", len(checks))
	}

	text := ui.OutputWriter.String()
	if !strings.Contains(text, "Checking production again") {
		t.Errorf("expected a re-check:\n%s", text)
	}
	if !strings.Contains(ui.ErrorWriter.String(), "Back up your databases before upgrading MariaDB") {
		t.Errorf("expected the backup note:\n%s", ui.ErrorWriter.String())
	}
}

func TestDoctorFixDeclined(t *testing.T) {
	output := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, output)

	ui := cli.NewMockUi()
	code := newFixCommand(ui, "n\n").Run([]string{"--offline", "--fix", "production"})

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if provisions := playbookCalls(*calls, "server.yml"); len(provisions) != 0 {
		t.Errorf("expected no provision run, got %v", provisions)
	}
	if !strings.Contains(ui.OutputWriter.String(), "Skipped production.") {
		t.Errorf("expected skipped message:\n%s", ui.OutputWriter.String())
	}
}

func TestDoctorFixNothingToDo(t *testing.T) {
	output := readDoctorFixture(t, "playbook_unreachable.json")
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, output)

	ui := cli.NewMockUi()
	newFixCommand(ui, "").Run([]string{"--offline", "--fix", "production"})

	if provisions := playbookCalls(*calls, "server.yml"); len(provisions) != 0 {
		t.Errorf("expected no provision run, got %v", provisions)
	}
	if !strings.Contains(ui.OutputWriter.String(), "--fix has nothing to run.") {
		t.Errorf("expected nothing-to-do message:\n%s", ui.OutputWriter.String())
	}
}

func TestDoctorFixValidation(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, "")

	ui := cli.NewMockUi()
	if code := newFixCommand(ui, "").Run([]string{"--fix", "--json", "production"}); code != 2 {
		t.Errorf("--fix --json: expected exit code 2, got %d", code)
	}

	ui = cli.NewMockUi()
	c := NewDoctorCommand(ui, trellis.NewTrellis(), "1.0.0")
	c.stdinIsTerminal = func() bool { return false }
	if code := c.Run([]string{"--fix", "production"}); code != 2 {
		t.Errorf("--fix without a terminal: expected exit code 2, got %d", code)
	}
	if !strings.Contains(ui.ErrorWriter.String(), "needs a terminal") {
		t.Errorf("expected terminal error:\n%s", ui.ErrorWriter.String())
	}

	if len(*calls) != 0 {
		t.Errorf("expected nothing to run, got %v", *calls)
	}
}

// healthyPlaybookOutput turns the outdated fixture into one where every
// package is current.
func healthyPlaybookOutput(outdated string) string {
	return strings.ReplaceAll(outdated, ".cmp=lt", ".cmp=eq")
}

func TestDoctorFixExitCodeFromRecheck(t *testing.T) {
	outdated := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	mockAnsible(t, []string{outdated, healthyPlaybookOutput(outdated)}, 0)

	ui := cli.NewMockUi()
	code := newFixCommand(ui, "y\n").Run([]string{"--offline", "--fix", "production"})

	if code != 0 {
		t.Errorf("expected exit code 0 after a successful fix, got %d\n%s", code, ui.OutputWriter.String())
	}
}

func TestDoctorFixProvisionFailure(t *testing.T) {
	outdated := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	mockAnsible(t, []string{outdated, healthyPlaybookOutput(outdated)}, 2)

	ui := cli.NewMockUi()
	code := newFixCommand(ui, "y\n").Run([]string{"--offline", "--fix", "production"})

	if code != 1 {
		t.Errorf("expected exit code 1 when provisioning fails, got %d", code)
	}
	if !strings.Contains(ui.ErrorWriter.String(), "Provisioning production failed") {
		t.Errorf("expected failure message:\n%s", ui.ErrorWriter.String())
	}
}

func TestDoctorFixOnlyRunsOnYes(t *testing.T) {
	output := readDoctorFixture(t, "playbook_ok.json")

	for name, answer := range map[string]string{"empty": "\n", "other": "maybe\n", "eof": ""} {
		t.Run(name, func(t *testing.T) {
			defer trellis.LoadFixtureProject(t)()
			calls := mockAnsiblePlaybook(t, output)

			ui := cli.NewMockUi()
			newFixCommand(ui, answer).Run([]string{"--offline", "--fix", "production"})

			if provisions := playbookCalls(*calls, "server.yml"); len(provisions) != 0 {
				t.Errorf("expected no provision run for answer %q, got %v", answer, provisions)
			}
		})
	}
}

func TestDoctorFixDevelopmentKeepsHostKeyChecking(t *testing.T) {
	output := readDoctorFixture(t, "playbook_ok.json")
	defer trellis.LoadFixtureProject(t)()
	calls := mockAnsiblePlaybook(t, output)
	t.Setenv("ANSIBLE_HOST_KEY_CHECKING", "")

	ui := cli.NewMockUi()
	newFixCommand(ui, "y\n").Run([]string{"--offline", "--fix", "development"})

	if provisions := playbookCalls(*calls, "dev.yml"); len(provisions) != 1 {
		t.Fatalf("expected development to be provisioned, got %v", *calls)
	}
	if v := os.Getenv("ANSIBLE_HOST_KEY_CHECKING"); v != "" {
		t.Errorf("provisioning development changed the process environment: ANSIBLE_HOST_KEY_CHECKING=%q", v)
	}
}
