package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestLogsRunValidations(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	cases := []struct {
		name            string
		projectDetected bool
		args            []string
		out             string
		code            int
	}{
		{
			"no_project",
			false,
			nil,
			"No Trellis project detected",
			1,
		},
		{
			"too_many_args",
			true,
			[]string{"development", "example.com", "bar"},
			"Error: too many arguments",
			1,
		},
		{
			"invalid_env",
			true,
			[]string{"foo"},
			"Error: foo is not a valid environment",
			1,
		},
		{
			"invalid_site",
			true,
			[]string{"development", "foo"},
			"Error: foo is not a valid site",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			logsCommand := NewLogsCommand(ui, trellis)

			code := logsCommand.Run(tc.args)

			if code != tc.code {
				t.Errorf("expected code %d to be %d", code, tc.code)
			}

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}
		})
	}
}

func TestLogsRun(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"default",
			[]string{"development"},
			"ssh vagrant@example.test tail -f /srv/www/example.com/logs/*[^gz]?",
			0,
		},
		{
			"access option",
			[]string{"--access", "development"},
			"ssh vagrant@example.test tail -f /srv/www/example.com/logs/access.log",
			0,
		},
		{
			"error option",
			[]string{"--error", "development"},
			"ssh vagrant@example.test tail -f /srv/www/example.com/logs/error.log",
			0,
		},
		{
			"number option",
			[]string{"--number=10", "development"},
			"ssh vagrant@example.test tail -n 10 -f /srv/www/example.com/logs/*[^gz]?",
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			logsCommand := NewLogsCommand(ui, trellis)
			code := logsCommand.Run(tc.args)

			if code != tc.code {
				t.Errorf("expected code %d to be %d", code, tc.code)
			}

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}
		})
	}
}

func TestLogsRunGoAccess(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()

	tmpDir := t.TempDir()

	// fake goaccess binary to satisfy ok.LookPath
	goAccessPath := filepath.Join(tmpDir, "goaccess")
	os.OpenFile(goAccessPath, os.O_CREATE, 0555)
	path := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("PATH=%s:%s", path, tmpDir))

	cases := []struct {
		name string
		args []string
		out  []string
		code int
	}{
		{
			"goaccess",
			[]string{"--goaccess", "development"},
			[]string{
				"ssh vagrant@example.test tail -n +0 -f /srv/www/example.com/logs/*",
				"goaccess --log-format=COMBINED",
			},
			0,
		},
		{
			"goaccess flags",
			[]string{"--goaccess", "--goaccess-flags=-a -m", "development"},
			[]string{
				"ssh vagrant@example.test tail -n +0 -f /srv/www/example.com/logs/*",
				"goaccess --log-format=COMBINED -a -m",
			},
			0,
		},
		{
			"goaccess with access only",
			[]string{"--goaccess", "--access", "development"},
			[]string{
				"ssh vagrant@example.test tail -n +0 -f /srv/www/example.com/logs/access.log",
				"goaccess --log-format=COMBINED",
			},
			0,
		},
		{
			"goaccess with error only",
			[]string{"--goaccess", "--error", "development"},
			[]string{
				"ssh vagrant@example.test tail -n +0 -f /srv/www/example.com/logs/error.log",
				"goaccess --log-format=COMBINED",
			},
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			logsCommand := NewLogsCommand(ui, trellis)
			code := logsCommand.Run(tc.args)

			if code != tc.code {
				t.Errorf("%s - actual code %d expected to be %d", tc.name, code, tc.code)
			}

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			for _, out := range tc.out {
				if !strings.Contains(combined, out) {
					t.Errorf("%s - actual output %q expected to contain %q", tc.name, combined, out)
				}
			}
		})
	}
}

func TestLogsHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	switch os.Args[3] {
	case "ssh":
		fmt.Fprintf(os.Stdout, "nginx log")
		os.Exit(0)
	case "tail":
		os.Exit(0)
	default:
		t.Fatalf("unexpected command %s", os.Args[3])
	}
}
