package trellis

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/trellis/yamledit"
)

type MockStringGenerator struct{}

func (ms *MockStringGenerator) Generate() string {
	return "random"
}

const productionVault = `# Documentation: https://roots.io/trellis/docs/vault/
vault_mysql_root_password: productionpw

# Documentation: https://roots.io/trellis/docs/security/
vault_users:
  - name: "{{ admin_user }}"
    password: example_password
    salt: "generateme"

vault_wordpress_sites:
  example.com:
    env:
      db_password: my_real_password
      # Generate your keys here: https://roots.io/salts.html
      auth_key: "generateme"
      secure_auth_key: "generateme"
      logged_in_key: "generateme"
      nonce_key: "generateme"
      auth_salt: "generateme"
      secure_auth_salt: "generateme"
      logged_in_salt: "generateme"
      nonce_salt: ""
`

func TestGenerateVaultSecrets(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		env      string
		sites    []string
		opts     VaultGenerateOptions
		expected string
	}{
		{
			"empty_development",
			"",
			"development",
			[]string{"testsite.com"},
			VaultGenerateOptions{},
			`
vault_mysql_root_password: random
vault_wordpress_sites:
  testsite.com:
    admin_password: random
    env:
      db_password: random
      auth_key: random
      secure_auth_key: random
      logged_in_key: random
      nonce_key: random
      auth_salt: random
      secure_auth_salt: random
      logged_in_salt: random
      nonce_salt: random
`,
		},
		{
			"empty_production",
			"",
			"production",
			[]string{"testsite.com"},
			VaultGenerateOptions{},
			`
vault_mysql_root_password: random
vault_users:
  - name: '{{ admin_user }}'
    password: random
    salt: random
vault_wordpress_sites:
  testsite.com:
    env:
      db_password: random
      auth_key: random
      secure_auth_key: random
      logged_in_key: random
      nonce_key: random
      auth_salt: random
      secure_auth_salt: random
      logged_in_salt: random
      nonce_salt: random
`,
		},
		{
			"fills_placeholders_only",
			productionVault,
			"production",
			[]string{"example.com"},
			VaultGenerateOptions{},
			`
# Documentation: https://roots.io/trellis/docs/vault/
vault_mysql_root_password: random

# Documentation: https://roots.io/trellis/docs/security/
vault_users:
  - name: "{{ admin_user }}"
    password: random
    salt: "random"

vault_wordpress_sites:
  example.com:
    env:
      db_password: my_real_password
      # Generate your keys here: https://roots.io/salts.html
      auth_key: "random"
      secure_auth_key: "random"
      logged_in_key: "random"
      nonce_key: "random"
      auth_salt: "random"
      secure_auth_salt: "random"
      logged_in_salt: "random"
      nonce_salt: "random"
`,
		},
		{
			"salts_only",
			productionVault,
			"production",
			[]string{"example.com"},
			VaultGenerateOptions{SaltsOnly: true},
			`
# Documentation: https://roots.io/trellis/docs/vault/
vault_mysql_root_password: productionpw

# Documentation: https://roots.io/trellis/docs/security/
vault_users:
  - name: "{{ admin_user }}"
    password: example_password
    salt: "generateme"

vault_wordpress_sites:
  example.com:
    env:
      db_password: my_real_password
      # Generate your keys here: https://roots.io/salts.html
      auth_key: "random"
      secure_auth_key: "random"
      logged_in_key: "random"
      nonce_key: "random"
      auth_salt: "random"
      secure_auth_salt: "random"
      logged_in_salt: "random"
      nonce_salt: "random"
`,
		},
		{
			"force_replaces_set_values_but_not_templates",
			`
vault_mysql_root_password: "{{ lookup('env', 'MYSQL_ROOT') }}"
vault_wordpress_sites:
  example.com:
    admin_password: secret
    env:
      db_password: my_real_password
`,
			"development",
			[]string{"example.com"},
			VaultGenerateOptions{Force: true, SaltsOnly: false},
			`
vault_mysql_root_password: "{{ lookup('env', 'MYSQL_ROOT') }}"
vault_wordpress_sites:
  example.com:
    admin_password: random
    env:
      db_password: random
      auth_key: random
      secure_auth_key: random
      logged_in_key: random
      nonce_key: random
      auth_salt: random
      secure_auth_salt: random
      logged_in_salt: random
      nonce_salt: random
`,
		},
		{
			"multiple_sites_and_non_sequence_users",
			`
vault_mysql_root_password: set
vault_users: "{{ my_users }}"
`,
			"production",
			[]string{"a.com", "b.com"},
			VaultGenerateOptions{SaltsOnly: true},
			`
vault_mysql_root_password: set
vault_users: "{{ my_users }}"
vault_wordpress_sites:
  a.com:
    env:
      auth_key: random
      secure_auth_key: random
      logged_in_key: random
      nonce_key: random
      auth_salt: random
      secure_auth_salt: random
      logged_in_salt: random
      nonce_salt: random
  b.com:
    env:
      auth_key: random
      secure_auth_key: random
      logged_in_key: random
      nonce_key: random
      auth_salt: random
      secure_auth_salt: random
      logged_in_salt: random
      nonce_salt: random
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc, err := yamledit.Parse([]byte(strings.TrimLeft(tc.input, "\n")))
			if err != nil {
				t.Fatal(err)
			}

			if _, err := GenerateVaultSecrets(doc, tc.env, tc.sites, &MockStringGenerator{}, tc.opts); err != nil {
				t.Fatal(err)
			}

			output, _ := doc.Bytes()
			expected := strings.TrimSpace(tc.expected)

			if strings.TrimSpace(string(output)) != expected {
				t.Errorf("expected:\n%s\ngot:\n%s", expected, output)
			}
		})
	}
}

func TestGenerateVaultSecretsReturnsGeneratedPaths(t *testing.T) {
	doc, _ := yamledit.Parse([]byte(productionVault))

	generated, err := GenerateVaultSecrets(doc, "production", []string{"example.com"}, &MockStringGenerator{}, VaultGenerateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	// root password, user password + salt, 8 keys/salts (db_password is already set)
	if len(generated) != 11 {
		t.Errorf("expected 11 generated values, got %d: %v", len(generated), generated)
	}

	generated, _ = GenerateVaultSecrets(doc, "production", []string{"example.com"}, &MockStringGenerator{}, VaultGenerateOptions{})
	if len(generated) != 0 {
		t.Errorf("expected second run to generate nothing, got %v", generated)
	}
}

func TestUpdateDefaultVaultFile(t *testing.T) {
	trellis := &Trellis{Path: t.TempDir()}

	data, _ := os.ReadFile("testdata/trellis/group_vars/development/vault.yml")
	if err := os.WriteFile(filepath.Join(trellis.Path, "vault.yml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	if err := trellis.UpdateDefaultVaultFile("vault.yml", "mysite.com", "development", "# Created by trellis-cli v1.0.0"); err != nil {
		t.Fatal(err)
	}

	doc, err := yamledit.Load(filepath.Join(trellis.Path, "vault.yml"))
	if err != nil {
		t.Fatal(err)
	}

	if doc.Lookup("vault_wordpress_sites", DefaultSiteName) != nil {
		t.Error("expected default site to be renamed")
	}

	for _, path := range [][]string{
		{"vault_mysql_root_password"},
		{"vault_wordpress_sites", "mysite.com", "admin_password"},
		{"vault_wordpress_sites", "mysite.com", "env", "db_password"},
		{"vault_wordpress_sites", "mysite.com", "env", "nonce_salt"},
	} {
		if node := doc.Lookup(path...); node == nil || len(node.Value) != 64 {
			t.Errorf("expected %v to be a generated 64 character value, got %v", path, node)
		}
	}

	if !strings.HasPrefix(doc.HeadComment(), "# Created by trellis-cli v1.0.0") {
		t.Errorf("expected header comment, got %q", doc.HeadComment())
	}

	output, _ := os.ReadFile(filepath.Join(trellis.Path, "vault.yml"))
	if !strings.Contains(string(output), "# Documentation: https://roots.io/trellis/docs/vault/") {
		t.Errorf("expected original comments to be preserved, got:\n%s", output)
	}
}

func TestUpdateDefaultVaultFileMissing(t *testing.T) {
	trellis := &Trellis{Path: t.TempDir()}

	if err := trellis.UpdateDefaultVaultFile("vault.yml", "mysite.com", "production", ""); err != nil {
		t.Fatal(err)
	}

	doc, err := yamledit.Load(filepath.Join(trellis.Path, "vault.yml"))
	if err != nil {
		t.Fatal(err)
	}

	if node := doc.Lookup("vault_users", "0", "salt"); node == nil || len(node.Value) != 64 {
		t.Errorf("expected vault_users to be generated, got %v", node)
	}
}
