package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestDotEnvArgumentValidations(t *testing.T) {
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
			[]string{"foo", "bar"},
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)

			dotEnvCommand := NewDotEnvCommand(ui, trellis)
			code := dotEnvCommand.Run(tc.args)

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

func TestDotEnvRun(t *testing.T) {
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
			nil,
			"ansible-playbook dotenv.yml -e env=development",
			0,
		},
		{
			"custom_env",
			[]string{"production"},
			"ansible-playbook dotenv.yml -e env=production",
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			dotEnvCommand := NewDotEnvCommand(ui, trellis)
			code := dotEnvCommand.Run(tc.args)

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}

			if code != tc.code {
				t.Errorf("expected code %d to be %d", code, tc.code)
			}
		})
	}
}

func TestIntegrationDotEnv(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	bin := os.Getenv("TEST_BINARY")
	if bin == "" {
		t.Error("TEST_BINARY not supplied")
	}
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Error(bin + " not exist")
	}

	dummy := os.Getenv("TEST_DUMMY")
	if dummy == "" {
		t.Error("TEST_DUMMY not supplied")
	}

	actualPath := filepath.Join(dummy, "site/.env")

	os.Remove(actualPath)
	defer os.Remove(actualPath)

	dotEnv := exec.Command(bin, "dotenv")
	dotEnv.Dir = filepath.Join(dummy, "trellis")

	dotEnv.Run()

	if _, err := os.Stat(actualPath); os.IsNotExist(err) {
		t.Error(".env file not generated")
	}

	actualByte, _ := os.ReadFile(actualPath)
	actual := string(actualByte)

	expectedByte, _ := os.ReadFile("./testdata/expected/dot_env/.env")
	expected := string(expectedByte)

	if actual != expected {
		t.Errorf("expected .env file \n%s to be \n%s", actual, expected)
	}
}
