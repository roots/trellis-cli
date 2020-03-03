package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func TestShellInitRunValidations(t *testing.T) {
	ui := cli.NewMockUi()

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"no_args",
			nil,
			"Error: missing arguments (expected exactly 1, got 0)",
			1,
		},
		{
			"too_many_args",
			[]string{"bash", "zsh"},
			"Error: too many arguments",
			1,
		},
		{
			"invalid_shell",
			[]string{"foo"},
			"Error: invalid shell name 'foo'. Supported shells: bash, zsh",
			1,
		},
	}

	for _, tc := range cases {
		shellInitCommand := &ShellInitCommand{ui}
		code := shellInitCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestShellInitRun(t *testing.T) {
	ui := cli.NewMockUi()
	shellInitCommand := &ShellInitCommand{ui}
	executable, _ := os.Executable()

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"bash",
			[]string{"bash"},
			executable,
			0,
		},
		{
			"zsh",
			[]string{"zsh"},
			executable,
			0,
		},
	}

	for _, tc := range cases {
		code := shellInitCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}
