package cmd

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

func TestSshRunValidations(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
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
			"no_args",
			true,
			nil,
			"Usage: trellis",
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
			"invalid_env_with_site",
			true,
			[]string{"foo", "example.com"},
			"Error: foo is not a valid environment",
			1,
		},
		{
			"invalid_site",
			true,
			[]string{"development", "nosite"},
			"Error: nosite is not a valid site",
			1,
		},
		{
			"too_many_args",
			true,
			[]string{"development", "site", "foo"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		mockCmd := &MockCommand{}
		mockProject := &MockProject{tc.projectDetected}
		trellis := trellis.NewTrellis(mockProject)
		sshCommand := &SshCommand{ui, trellis, &MockCommandExecutor{mockCmd}}

		code := sshCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestSshRun(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	ui := cli.NewMockUi()

	cases := []struct {
		name    string
		args    []string
		runCmd  string
		runArgs string
		code    int
	}{
		{
			"non_development",
			[]string{"production"},
			"ssh",
			"ssh admin@example.com",
			0,
		},
		{
			"development",
			[]string{"development"},
			"ssh",
			"ssh vagrant@example.test",
			0,
		},
	}

	for _, tc := range cases {
		mockCmd := &MockCommand{}
		project := &trellis.Project{}
		trellis := trellis.NewTrellis(project)
		sshCommand := &SshCommand{ui, trellis, &MockCommandExecutor{mockCmd}}

		code := sshCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		if tc.runCmd != mockCmd.cmd {
			t.Errorf("expected command %q to contain %q", mockCmd.cmd, tc.runCmd)
		}

		if tc.runArgs != mockCmd.args {
			t.Errorf("expected args %s to be %s", mockCmd.args, tc.runArgs)
		}
	}
}
