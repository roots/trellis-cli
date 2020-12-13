package cmd

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestVaultEncryptRunValidations(t *testing.T) {
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
		vaultEncryptCommand := NewVaultEncryptCommand(ui, trellis)

		code := vaultEncryptCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestVaultEncryptRun(t *testing.T) {
	defer MockExec(t)()

	ui := cli.NewMockUi()
	project := &trellis.Project{}
	trellisProject := trellis.NewTrellis(project)
	vaultEncryptCommand := NewVaultEncryptCommand(ui, trellisProject)

	defer trellis.TestChdir(t, "../trellis/testdata/trellis")()

	if err := trellisProject.LoadProject(); err != nil {
		t.Fatalf(err.Error())
	}

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"default",
			[]string{"production"},
			"ansible-vault encrypt group_vars/all/vault.yml group_vars/production/vault.yml",
			0,
		},
		{
			"files_flag_single_file",
			[]string{"--files=group_vars/production/vault.yml", "production"},
			"ansible-vault encrypt group_vars/production/vault.yml",
			0,
		},
		{
			"files_flag_multiple_file",
			[]string{"--files=group_vars/production/vault.yml,group_vars/development/vault.yml", "production"},
			"ansible-vault encrypt group_vars/production/vault.yml group_vars/development/vault.yml",
			0,
		},
		{
			"already_encrypted_file",
			[]string{"--files=group_vars/production/encrypted.yml", "production"},
			"All files already encrypted",
			0,
		},
	}

	for _, tc := range cases {
		code := vaultEncryptCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}
