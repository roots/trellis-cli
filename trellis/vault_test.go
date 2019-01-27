package trellis

import (
	"gopkg.in/yaml.v2"
	"strings"
	"testing"
)

type MockStringGenerator struct{}

func (ms *MockStringGenerator) Generate() string {
	return "random"
}

func TestGenerateVaultConfig(t *testing.T) {
	trellis := &Trellis{}
	mockStringGenerator := &MockStringGenerator{}

	cases := []struct {
		name         string
		siteName     string
		env          string
		expectedYaml string
	}{
		{
			"development_env",
			"testsite.com",
			"development",
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
			"production_env",
			"testsite.com",
			"production",
			`
vault_mysql_root_password: random
vault_users:
- name: '{{ admin_user }}'
  password: random
  salt: random
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
	}

	for _, tc := range cases {
		vault := trellis.GenerateVaultConfig(tc.siteName, tc.env, mockStringGenerator)

		vaultYaml, _ := yaml.Marshal(vault)
		expected := strings.TrimSpace(tc.expectedYaml)
		output := strings.TrimSpace(string(vaultYaml))

		if output != expected {
			t.Errorf("expected yaml output %s but got %s", expected, output)
		}
	}
}
