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
		siteName     string
		host         string
		expectedYaml string
	}{
		{
			"development_env",
			"development",
			"testsite",
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
			"testsite",
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
			"testsite",
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
		{
			"default_site_name_clash",
			"production",
			"example.com",
			"example.com",
			`
wordpress_sites:
  example.com:
    site_hosts:
    - canonical: example.com
      redirects:
      - www.example.com
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
		{
			"www_domain",
			"production",
			"www.example.com",
			"www.example.com",
			`
wordpress_sites:
  www.example.com:
    site_hosts:
    - canonical: www.example.com
      redirects:
      - example.com
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
		{
			"non_com_tld",
			"production",
			"example.co.uk",
			"www.example.co.uk",
			`
wordpress_sites:
  example.co.uk:
    site_hosts:
    - canonical: www.example.co.uk
      redirects:
      - example.co.uk
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
		{
			"subdomain",
			"production",
			"foo.example.com",
			"foo.example.com",
			`
wordpress_sites:
  foo.example.com:
    site_hosts:
    - canonical: foo.example.com
      redirects: []
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
		{
			"development_env_www_domain",
			"development",
			"www.example.com",
			"www.example.com",
			`
wordpress_sites:
  www.example.com:
    site_hosts:
    - canonical: www.example.test
      redirects:
      - example.test
    local_path: ../site
    admin_email: admin@www.example.test
    multisite:
      enabled: false
    ssl:
      enabled: false
      provider: self-signed
    cache:
      enabled: false
`,
		},
	}

	for _, tc := range cases {
		config := trellis.ParseConfig(fmt.Sprintf("testdata/trellis/group_vars/%s/wordpress_sites.yml", tc.env))
		trellis.UpdateDefaultConfig(config, tc.siteName, tc.host, tc.env)

		configYaml, _ := yaml.Marshal(config)
		expected := strings.TrimSpace(tc.expectedYaml)
		output := strings.TrimSpace(string(configYaml))

		if output != expected {
			t.Errorf("%s => expected yaml output \n%s\n but got \n%s", tc.name, expected, output)
		}
	}
}
