package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
	"github.com/roots/trellis-cli/trellis/yamledit"
)

func TestVaultGenerateRunValidations(t *testing.T) {
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
			[]string{"production", "foo"},
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
			"force_without_env",
			true,
			[]string{"--force"},
			"Error: the --force option requires an ENVIRONMENT argument",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var project *trellis.Trellis
			if tc.projectDetected {
				project = trellis.NewTrellis()
			} else {
				project = trellis.NewMockTrellis(false)
			}

			ui := cli.NewMockUi()
			code := NewVaultGenerateCommand(ui, project).Run(tc.args)

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

func TestVaultGenerateRun(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	ui := cli.NewMockUi()
	code := NewVaultGenerateCommand(ui, trellis.NewTrellis()).Run([]string{})

	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, ui.ErrorWriter.String())
	}

	output := ui.OutputWriter.String()

	for _, expected := range []string{
		"Generated 10 secrets in group_vars/development/vault.yml",
		"Generated 12 secrets in group_vars/production/vault.yml",
		"Generated 39 secrets in group_vars/valet-link/vault.yml",
	} {
		if !strings.Contains(output, expected) {
			t.Errorf("expected output %q to contain %q", output, expected)
		}
	}

	doc, err := yamledit.Load("group_vars/production/vault.yml")
	if err != nil {
		t.Fatal(err)
	}

	if node := doc.Lookup("vault_wordpress_sites", "example.com", "env", "auth_salt"); node == nil || len(node.Value) != 64 {
		t.Errorf("expected auth_salt to be generated, got %v", node)
	}

	data, _ := os.ReadFile("group_vars/production/vault.yml")
	if !strings.Contains(string(data), "# Generate your keys here: https://roots.io/salts.html") {
		t.Errorf("expected comments to be preserved, got:\n%s", data)
	}

	ui = cli.NewMockUi()
	NewVaultGenerateCommand(ui, trellis.NewTrellis()).Run([]string{"production"})

	if !strings.Contains(ui.OutputWriter.String(), "No secrets to generate in group_vars/production/vault.yml") {
		t.Errorf("expected second run to generate nothing, got %q", ui.OutputWriter.String())
	}
}

func TestVaultGenerateRunEncrypted(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	path := "group_vars/production/vault.yml"
	plaintext, _ := os.ReadFile(path)

	if err := os.WriteFile(path, []byte("$ANSIBLE_VAULT;1.1;AES256\nencrypted\n"), 0644); err != nil {
		t.Fatal(err)
	}

	defer command.MockExecCommands(t, []command.MockCommand{
		{Command: "ansible-vault", Args: []string{"decrypt", "--output", "-", path}, Output: string(plaintext)},
		{Command: "ansible-vault", Args: []string{"encrypt", "--output", path}},
	})()

	ui := cli.NewMockUi()
	code := NewVaultGenerateCommand(ui, trellis.NewTrellis()).Run([]string{"production"})

	if code != 0 {
		t.Fatalf("expected code 0, got %d: %s", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), "Generated 12 secrets in "+path) {
		t.Errorf("unexpected output %q", ui.OutputWriter.String())
	}

	data, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(data), "$ANSIBLE_VAULT") {
		t.Errorf("expected vault file to never be written in plaintext, got:\n%s", data)
	}
}

func TestVaultGenerateRunDecryptFailure(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	path := "group_vars/production/vault.yml"
	if err := os.WriteFile(path, []byte("$ANSIBLE_VAULT;1.1;AES256\nencrypted\n"), 0644); err != nil {
		t.Fatal(err)
	}

	defer command.MockExecCommands(t, []command.MockCommand{
		{Command: "ansible-vault", Args: []string{"decrypt", "--output", "-", path}, ExitCode: 1},
	})()

	ui := cli.NewMockUi()
	code := NewVaultGenerateCommand(ui, trellis.NewTrellis()).Run([]string{"production"})

	if code != 1 {
		t.Errorf("expected code 1, got %d", code)
	}

	if !strings.Contains(ui.ErrorWriter.String(), "could not decrypt "+path) {
		t.Errorf("unexpected error output %q", ui.ErrorWriter.String())
	}
}
