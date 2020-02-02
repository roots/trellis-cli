package cmd

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

func TestVaultEditRunValidations(t *testing.T) {
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
			[]string{"group_vars/all/vault.yml", "group_vars/production/vault.yml"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		mockCmd := &MockCommand{}
		mockProject := &MockProject{tc.projectDetected}
		trellis := trellis.NewTrellis(mockProject)
		vaultEditCommand := NewVaultEditCommand(ui, trellis, &MockCommandExecutor{mockCmd})

		code := vaultEditCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestVaultEditRun(t *testing.T) {
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
			"default",
			[]string{"group_vars/development/vault.yml"},
			"ansible-vault",
			"ansible-vault edit group_vars/development/vault.yml",
			0,
		},
	}

	for _, tc := range cases {
		mockCmd := &MockCommand{}
		project := &trellis.Project{}
		trellis := trellis.NewTrellis(project)
		vaultEditCommand := NewVaultEditCommand(ui, trellis, &MockCommandExecutor{mockCmd})

		code := vaultEditCommand.Run(tc.args)

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
