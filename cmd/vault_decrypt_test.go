package cmd

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestVaultDecryptRunValidations(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			vaultDecryptCommand := NewVaultDecryptCommand(ui, trellis)

			code := vaultDecryptCommand.Run(tc.args)

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

func TestVaultDecryptRun(t *testing.T) {
	trellisProject := trellis.NewTrellis()

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
			"environment_with_files",
			[]string{"-f=foo", "production"},
			"Error: the file option can't be used together with the ENVIRONMENT argument",
			1,
		},
		{
			"default",
			[]string{"production"},
			"All files already decrypted",
			0,
		},
		{
			"files_flag_single_file",
			[]string{"-f=group_vars/production/vault.yml"},
			"All files already decrypted",
			0,
		},
		{
			"files_flag_multiple_file",
			[]string{"-f=group_vars/production/vault.yml", "-f=group_vars/development/vault.yml"},
			"All files already decrypted",
			0,
		},
		{
			"already_decrypted_file",
			[]string{"-f=group_vars/production/encrypted.yml"},
			"ansible-vault decrypt group_vars/production/encrypted.yml",
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			vaultDecryptCommand := NewVaultDecryptCommand(ui, trellisProject)
			code := vaultDecryptCommand.Run(tc.args)

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
