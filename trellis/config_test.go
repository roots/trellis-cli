package trellis

import (
	"fmt"
	"gopkg.in/yaml.v2"
	"strings"
	"testing"
)

func TestUpdateDefaultConfig(t *testing.T) {
	trellis := &Trellis{}

	cases := []struct {
		name         string
		env          string
		host         string
		expectedYaml string
	}{
		{
			"development_env",
			"development",
			"testsite.com",
			`
wordpress_sites:
  testsite:
    site_hosts:
    - canonical: testsite.test
      redirects:
      - www.testsite.test
    local_path: ../site
    admin_email: admin@testsite.test
    multisite:
      enabled: false
    ssl:
      enabled: false
      provider: self-signed
    cache:
      enabled: false
`,
		},
		{
			"development_env_www_host",
			"development",
			"www.testsite.com",
			`
wordpress_sites:
  testsite:
    site_hosts:
    - canonical: www.testsite.test
      redirects:
      - testsite.test
    local_path: ../site
    admin_email: admin@www.testsite.test
    multisite:
      enabled: false
    ssl:
      enabled: false
      provider: self-signed
    cache:
      enabled: false
`,
		},
		{
			"production_env",
			"production",
			"testsite.com",
			`
wordpress_sites:
  testsite:
    site_hosts:
    - canonical: testsite.com
      redirects:
      - www.testsite.com
    local_path: ../site
    branch: master
    repo: git@github.com:example/example.com.git
    repo_subtree_path: site
    multisite:
      enabled: false
    ssl:
      enabled: false
      provider: letsencrypt
    cache:
      enabled: false
`,
		},
	}

	for _, tc := range cases {
		config := trellis.ParseConfig(fmt.Sprintf("../test-fixtures/trellis/group_vars/%s/wordpress_sites.yml", tc.env))
		trellis.UpdateDefaultConfig(config, "testsite", tc.host, tc.env)

		configYaml, _ := yaml.Marshal(config)
		expected := strings.TrimSpace(tc.expectedYaml)
		output := strings.TrimSpace(string(configYaml))

		if output != expected {
			t.Errorf("expected yaml output %s but got %s", expected, output)
		}
	}
}
