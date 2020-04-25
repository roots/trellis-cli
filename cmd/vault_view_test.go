package cmd

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

func TestVaultViewRunValidations(t *testing.T) {
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
			[]string{"production", "foo"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		mockProject := &MockProject{tc.projectDetected}
		trellis := trellis.NewTrellis(mockProject)
		vaultViewCommand := NewVaultViewCommand(ui, trellis)

		code := vaultViewCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestVaultViewRun(t *testing.T) {
	ui := cli.NewMockUi()
	project := &trellis.Project{}
	trellisProject := trellis.NewTrellis(project)

	defer trellis.TestChdir(t, "../trellis/testdata/trellis")()

	if err := trellisProject.LoadProject(); err != nil {
		t.Fatalf(err.Error())
	}

	defer MockExec(t)()

	mockProject := &MockProject{true}
	trellis := trellis.NewTrellis(mockProject)
	vaultViewCommand := NewVaultViewCommand(ui, trellis)

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"default",
			[]string{"production"},
			"ansible-vault view group_vars/all/vault.yml group_vars/production/vault.yml",
			0,
		},
		{
			"files_flag_single_file",
			[]string{"--files=foo", "production"},
			"ansible-vault view foo",
			0,
		},
		{
			"files_flag_multiple_file",
			[]string{"--files=foo,bar", "production"},
			"ansible-vault view foo bar",
			0,
		},
	}

	for _, tc := range cases {
		code := vaultViewCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}
