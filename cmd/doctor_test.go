package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/doctor"
	"github.com/roots/trellis-cli/trellis"
)

// mockAnsible makes doctor's playbook runs print outputs in turn (the last one
// repeats), as ansible-playbook with the json callback would, and every other
// command (such as provisioning) exit with otherExitCode. It records each
// command's args.
func mockAnsible(t *testing.T, outputs []string, otherExitCode int) *[][]string {
	t.Helper()

	calls := &[][]string{}
	doctorRuns := 0

	command.Mock(func(name string, args []string) *exec.Cmd {
		*calls = append(*calls, append([]string{name}, args...))

		output, exitCode := "", otherExitCode
		if len(args) > 0 && strings.HasPrefix(args[0], doctor.FilePrefix) {
			output, exitCode = outputs[min(doctorRuns, len(outputs)-1)], 0
			doctorRuns++
		}

		cmd := exec.Command(os.Args[0], "-test.run=TestDoctorHelperProcess", "--")
		cmd.Env = []string{
			"GO_WANT_DOCTOR_HELPER_PROCESS=1",
			"DOCTOR_HELPER_OUTPUT=" + output,
			"DOCTOR_HELPER_EXIT_CODE=" + strconv.Itoa(exitCode),
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

// healthyPlaybookOutput turns the outdated fixture into one where every
// package is current.
func healthyPlaybookOutput(outdated string) string {
	return strings.ReplaceAll(outdated, ".cmp=lt", ".cmp=eq")
}

func newTestDoctor(ui *cli.MockUi, terminal bool, answers string) *DoctorCommand {
	ui.InputReader = strings.NewReader(answers)
	c := NewDoctorCommand(ui, trellis.NewTrellis())
	c.stdinIsTerminal = func() bool { return terminal }
	return c
}

// playbookCalls returns the recorded commands running a playbook whose name
// starts with prefix, and the env each ran against.
func playbookCalls(calls [][]string, prefix string) (matching [][]string, envs []string) {
	for _, call := range calls {
		if len(call) > 1 && strings.HasPrefix(call[1], prefix) {
			matching = append(matching, call)
			for _, arg := range call {
				if env, ok := strings.CutPrefix(arg, "-e env="); ok {
					envs = append(envs, env)
				}
			}
		}
	}
	return matching, envs
}

func TestDoctorArgumentValidations(t *testing.T) {
	cases := []struct {
		name            string
		projectDetected bool
		terminal        bool
		args            []string
		out             string
	}{
		{"no project", false, true, nil, "No Trellis project detected"},
		{"invalid env", true, true, []string{"--offline", "nope"}, "Error: nope is not a valid environment"},
		{"option after env", true, true, []string{"production", "--quiet"}, "Options must come before environment names"},
		{"fix without a terminal", true, false, []string{"--fix", "production"}, "needs a terminal"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.projectDetected {
				defer trellis.LoadFixtureProject(t)()
			}
			calls := mockAnsible(t, []string{""}, 0)

			ui := cli.NewMockUi()
			c := newTestDoctor(ui, tc.terminal, "")
			if !tc.projectDetected {
				c.Trellis = trellis.NewMockTrellis(false)
			}

			if code := c.Run(tc.args); code != 2 {
				t.Errorf("expected exit code 2, got %d", code)
			}
			if combined := ui.OutputWriter.String() + ui.ErrorWriter.String(); !strings.Contains(combined, tc.out) {
				t.Errorf("expected %q in:\n%s", tc.out, combined)
			}
			if len(*calls) != 0 {
				t.Errorf("expected nothing to run, got %v", *calls)
			}
		})
	}
}

func TestDoctorRun(t *testing.T) {
	ok := readDoctorFixture(t, "playbook_ok.json")
	unreachable := readDoctorFixture(t, "playbook_unreachable.json")
	noHosts := readDoctorFixture(t, "playbook_no_hosts.json")

	cases := []struct {
		name   string
		setup  func(t *testing.T)
		output string
		args   []string
		code   int
		want   []string
		envs   []string
		// hint is the expected --fix hint; empty means none.
		hint string
	}{
		{
			name: "outdated", output: ok, args: []string{"production"}, code: 1, envs: []string{"production"},
			want: []string{"production · admin@default (127.0.0.1)", "✗ nginx", "Fix:", "trellis provision --tags nginx,php,mariadb"},
			hint: "Run `trellis doctor --offline --fix production` to have doctor run this for you.",
		},
		{
			name: "every environment except development", output: ok, code: 1,
			envs: []string{"production", "valet-link"},
			hint: "Run `trellis doctor --offline --fix production valet-link` to have doctor run these for you.",
		},
		{
			name: "inventory directory is left to Ansible", output: ok, args: []string{"production"}, code: 1,
			envs: []string{"production"},
			hint: "Run `trellis doctor --offline --fix production` to have doctor run this for you.",
			setup: func(t *testing.T) {
				if err := os.Remove("hosts/production"); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll("hosts/production", 0755); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "quiet has no --fix hint", output: ok, args: []string{"--quiet", "production"}, code: 1, envs: []string{"production"},
			want: []string{"Fix:"},
		},
		{
			name: "placeholder host", output: ok, args: []string{"production"}, code: 2,
			want: []string{"? couldn't check: hosts/production still has the placeholder host"},
			setup: func(t *testing.T) {
				placeholder := "[production]\nyour_server_hostname\n\n[web]\nyour_server_hostname\n"
				if err := os.WriteFile("hosts/production", []byte(placeholder), 0644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unreachable", output: unreachable, args: []string{"production"}, code: 2, envs: []string{"production"},
			want: []string{"Failed to connect to the host via ssh"},
		},
		{
			name: "no hosts", output: noHosts, args: []string{"production"}, code: 2, envs: []string{"production"},
			want: []string{"no hosts in both the [production] and [web] groups"},
		},
		{
			name: "ansible failure", output: "ERROR! Attempting to decrypt but no vault secrets found",
			args: []string{"production"}, code: 2, envs: []string{"production"},
			want: []string{"couldn't parse ansible-playbook output"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer trellis.LoadFixtureProject(t)()
			calls := mockAnsible(t, []string{tc.output}, 0)
			if tc.setup != nil {
				tc.setup(t)
			}

			ui := cli.NewMockUi()
			code := newTestDoctor(ui, true, "").Run(append([]string{"--offline"}, tc.args...))
			text := ui.OutputWriter.String()

			if code != tc.code {
				t.Errorf("expected exit code %d, got %d\n%s%s", tc.code, code, text, ui.ErrorWriter.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(text, want) {
					t.Errorf("expected %q in:\n%s", want, text)
				}
			}
			if hint := strings.Contains(text, "to have doctor run"); tc.hint == "" && hint {
				t.Errorf("expected no --fix hint in:\n%s", text)
			} else if tc.hint != "" && !strings.Contains(text, tc.hint) {
				t.Errorf("expected %q in:\n%s", tc.hint, text)
			}

			runs, envs := playbookCalls(*calls, doctor.FilePrefix)
			slices.Sort(envs)
			if !slices.Equal(envs, tc.envs) {
				t.Errorf("expected doctor to run on %v, got %v", tc.envs, envs)
			}
			for _, run := range runs {
				if !slices.ContainsFunc(run, func(arg string) bool { return strings.HasPrefix(arg, "-e "+doctor.ProbeVar+"="+doctor.FilePrefix) }) {
					t.Errorf("expected the probe file as an extra var, got %v", run)
				}
			}

			if leftovers, _ := filepath.Glob(doctor.FilePrefix + "*"); len(leftovers) > 0 {
				t.Errorf("expected temporary files to be removed, found %v", leftovers)
			}
		})
	}
}

func TestDoctorFix(t *testing.T) {
	outdated := readDoctorFixture(t, "playbook_ok.json")
	unreachable := readDoctorFixture(t, "playbook_unreachable.json")
	provisionCommand := []string{"ansible-playbook", "server.yml", "-e nginx_package_state=latest apt_package_state=latest mariadb_server_package_state=latest mariadb_client_package_state=latest", "--tags=nginx,php,mariadb", "-e env=production"}

	cases := []struct {
		name          string
		answer        string
		outputs       []string
		provisionExit int
		code          int
		provisioned   bool
		want          string
	}{
		{name: "yes, still outdated", answer: "y\n", outputs: []string{outdated}, code: 1, provisioned: true, want: "Checking production again"},
		{name: "yes, fixed", answer: "yes\n", outputs: []string{outdated, healthyPlaybookOutput(outdated)}, code: 0, provisioned: true},
		{name: "provision failed", answer: "y\n", outputs: []string{outdated, healthyPlaybookOutput(outdated)}, provisionExit: 2, code: 1, provisioned: true, want: "Provisioning production failed"},
		{name: "no", answer: "n\n", outputs: []string{outdated}, code: 1, want: "Skipped production."},
		{name: "empty answer", answer: "\n", outputs: []string{outdated}, code: 1},
		{name: "other answer", answer: "maybe\n", outputs: []string{outdated}, code: 1},
		{name: "eof", answer: "", outputs: []string{outdated}, code: 1, want: "Canceled"},
		{name: "nothing to run", outputs: []string{unreachable}, code: 2, want: "--fix has nothing to run."},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer trellis.LoadFixtureProject(t)()
			calls := mockAnsible(t, tc.outputs, tc.provisionExit)

			ui := cli.NewMockUi()
			code := newTestDoctor(ui, true, tc.answer).Run([]string{"--offline", "--fix", "production"})
			text := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if code != tc.code {
				t.Errorf("expected exit code %d, got %d\n%s", tc.code, code, text)
			}
			if tc.want != "" && !strings.Contains(text, tc.want) {
				t.Errorf("expected %q in:\n%s", tc.want, text)
			}
			if strings.Contains(text, "to have doctor run") {
				t.Errorf("expected no --fix hint when --fix was given:\n%s", text)
			}

			provisions, _ := playbookCalls(*calls, "server.yml")
			checks, _ := playbookCalls(*calls, doctor.FilePrefix)
			if tc.provisioned {
				if len(provisions) != 1 || !slices.Equal(provisions[0], provisionCommand) {
					t.Errorf("unexpected provision commands\ngot  %q\nwant %q", provisions, provisionCommand)
				}
				if len(checks) != 2 {
					t.Errorf("expected production to be checked again, got %d checks", len(checks))
				}
				if !strings.Contains(text, "Back up your databases before upgrading MariaDB") {
					t.Errorf("expected the backup note:\n%s", text)
				}
			} else if len(provisions) != 0 {
				t.Errorf("expected no provision run, got %v", provisions)
			}
		})
	}
}

func TestDoctorFixHint(t *testing.T) {
	report := &doctor.Report{Environments: []doctor.EnvReport{{Name: "production", Fix: &doctor.Fix{}}, {Name: "staging"}}}

	want := "Run `trellis doctor --fix production` to have doctor run this for you."
	if got := fixHint(report, false); got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
