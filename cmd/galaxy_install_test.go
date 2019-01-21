package cmd

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

func TestGalaxyInstallRunValidations(t *testing.T) {
	ui := cli.NewMockUi()

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
			[]string{"foo"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		mockProject := &MockProject{tc.projectDetected}
		trellis := trellis.NewTrellis(mockProject)
		galaxyInstallCommand := GalaxyInstallCommand{ui, trellis}

		code := galaxyInstallCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestGalaxyInstallRun(t *testing.T) {
	ui := cli.NewMockUi()
	mockProject := &MockProject{true}
	trellis := trellis.NewTrellis(mockProject)
	galaxyInstallCommand := GalaxyInstallCommand{ui, trellis}

	execCommand = mockExecCommand
	defer func() { execCommand = exec.Command }()

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"default",
			[]string{},
			"ansible-galaxy install -r requirements.yml",
			0,
		},
	}

	for _, tc := range cases {
		code := galaxyInstallCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}
