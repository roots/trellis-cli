package cmd

import (
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestProvisionRunValidations(t *testing.T) {
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
			"no_args",
			true,
			nil,
			"Error: missing arguments (expected exactly 1, got 0)",
			1,
		},
		{
			"too_many_args",
			true,
			[]string{"development", "foo"},
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
			"invalid_env",
			true,
			[]string{"--tags", "users", "-i", "development"},
			"Error: --interactive and --tags cannot be used together. Please use one or the other.",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			provisionCommand := NewProvisionCommand(ui, trellis)

			code := provisionCommand.Run(tc.args)

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

func TestProvisionInteractiveWithoutFzf(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()

	ui := cli.NewMockUi()
	defer MockUiExec(t, ui)()

	// Clear PATH to ensure fzf is not found
	t.Setenv("PATH", "")

	provisionCommand := NewProvisionCommand(ui, trellis)

	code := provisionCommand.Run([]string{"--interactive", "development"})

	expectedCode := 1

	if code != expectedCode {
		t.Errorf("expected code %d to be %d", code, expectedCode)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
	expectedOut := "Error: `fzf` command found. fzf is required to use interactive mode."
	if !strings.Contains(combined, expectedOut) {
		t.Errorf("expected output %q to contain %q", combined, expectedOut)
	}
}

func TestProvisionRun(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	trellis.CliConfig.Vm.Manager = "mock"

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"default",
			[]string{"development"},
			"ansible-playbook dev.yml -e env=development",
			0,
		},
		{
			"non_development",
			[]string{"production"},
			"ansible-playbook server.yml -e env=production",
			0,
		},
		{
			"with_tags",
			[]string{"-tags", "users", "development"},
			"ansible-playbook dev.yml --tags=users -e env=development",
			0,
		},
		{
			"with_extra_vars",
			[]string{"-extra-vars", "k=v foo=bar", "development"},
			"ansible-playbook dev.yml -e k=v foo=bar -e env=development",
			0,
		},
		{
			"with_verbose",
			[]string{"--verbose", "development"},
			"ansible-playbook dev.yml -vvvv -e env=development",
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			provisionCommand := NewProvisionCommand(ui, trellis)

			code := provisionCommand.Run(tc.args)

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
