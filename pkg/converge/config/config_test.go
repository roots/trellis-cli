package config

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func load(t *testing.T, project string) *Config {
	t.Helper()
	c, err := Load(filepath.Join("..", "testdata", "projects", project, FileName))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func resolve(t *testing.T, project, env string) *Environment {
	t.Helper()
	e, err := load(t, project).Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestMinimalDefaults(t *testing.T) {
	e := resolve(t, "minimal", "production")

	if got := e.Server.PHP.Version; got != "8.4" {
		t.Errorf("php version = %q", got)
	}
	if len(e.Nodes) != 1 || e.Nodes[0].Name != "203.0.113.10" {
		t.Fatalf("nodes = %+v", e.Nodes)
	}
	if diff := cmp.Diff([]string{RoleWeb, RoleDB}, e.Nodes[0].Roles); diff != "" {
		t.Errorf("roles: %s", diff)
	}
	s := e.Sites[0]
	if s.SSL != SSLLetsEncrypt || s.Branch != "main" || s.Cache != CacheOff {
		t.Errorf("site defaults not applied: %+v", s)
	}
	if diff := cmp.Diff([]string{"example.com", "www.example.com"}, s.Domains()); diff != "" {
		t.Error(diff)
	}
	if s.DBName() != "example_com_production" || s.DBUser() != "example_com" {
		t.Errorf("db name = %q", s.DBName())
	}
}

func TestDevelopmentImplicitNodeAndSelfSigned(t *testing.T) {
	e := resolve(t, "minimal", "development")
	if len(e.Nodes) != 1 || e.Nodes[0].Name != DevelopmentEnv {
		t.Fatalf("nodes = %+v", e.Nodes)
	}
	if e.Sites[0].SSL != SSLSelfSigned {
		t.Errorf("ssl = %q", e.Sites[0].SSL)
	}
}

func TestMergeRule(t *testing.T) {
	prod := resolve(t, "full", "production")
	staging := resolve(t, "full", "staging")

	// Maps deep-merge: env overrides max_children but keeps pm from defaults.
	if prod.Server.PHP.FPM.MaxChildren != 40 || prod.Server.PHP.FPM.PM != "dynamic" {
		t.Errorf("fpm = %+v", prod.Server.PHP.FPM)
	}
	if staging.Server.PHP.FPM.MaxChildren != 20 {
		t.Errorf("staging fpm = %+v", staging.Server.PHP.FPM)
	}
	// Defaults map keys survive.
	if prod.Server.PHP.INI["memory_limit"] != "256M" {
		t.Errorf("ini = %v", prod.Server.PHP.INI)
	}
	// null excludes a site from an environment.
	if len(staging.Sites) != 1 || staging.Sites[0].Name != "example.com" {
		t.Errorf("staging sites = %v", siteNames(staging))
	}
	if len(prod.Sites) != 2 {
		t.Errorf("prod sites = %v", siteNames(prod))
	}
	// Site fields merge with env overrides.
	if staging.Sites[0].Branch != "develop" || staging.Sites[0].Cache != CacheFastCGI {
		t.Errorf("staging site = %+v", staging.Sites[0])
	}
	// Multisite object form.
	shop := prod.Sites[1]
	if !shop.Multisite.Enabled || shop.Multisite.Mode != "subdomains" {
		t.Errorf("multisite = %+v", shop.Multisite)
	}
	// Scalars of any YAML type decode into string maps.
	if prod.Sites[0].PHP.INI["max_execution_time"] != "120" {
		t.Errorf("site ini = %v", prod.Sites[0].PHP.INI)
	}
}

func TestMergeFunction(t *testing.T) {
	base := map[string]any{
		"a": map[string]any{"x": 1, "y": 2},
		"l": []any{1, 2},
		"k": "keep",
		"d": "drop",
	}
	over := map[string]any{
		"a": map[string]any{"y": 3},
		"l": []any{9},
		"d": nil,
	}
	got := merge(base, over)
	want := map[string]any{
		"a": map[string]any{"x": 1, "y": 3},
		"l": []any{9},
		"k": "keep",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Error(diff)
	}
	// Inputs are not mutated.
	if _, ok := base["d"]; !ok {
		t.Error("merge mutated base")
	}
}

func TestSplitTopology(t *testing.T) {
	e := resolve(t, "split", "production")
	if len(e.Nodes) != 2 || !e.Nodes[0].HasRole(RoleWeb) || !e.Nodes[1].HasRole(RoleDB) {
		t.Fatalf("nodes = %+v", e.Nodes)
	}
	ext := resolve(t, "split", "staging")
	if ext.Database == nil || ext.Database.Port != 3306 {
		t.Fatalf("database = %+v", ext.Database)
	}
	if diff := cmp.Diff([]string{RoleWeb}, ext.Nodes[0].Roles); diff != "" {
		t.Errorf("external db node roles: %s", diff)
	}
}

func TestStrictUnknownKeys(t *testing.T) {
	_, err := Parse([]byte("version: 1\nsites:\n  a.com:\n    domian: a.com\n"))
	if err == nil || !strings.Contains(err.Error(), "domian") || !strings.Contains(err.Error(), "line 4") {
		t.Fatalf("expected unknown field error with line, got %v", err)
	}
}

func TestValidationErrors(t *testing.T) {
	cases := map[string]struct {
		yaml string
		want []string
	}{
		"missing domain and admins": {
			yaml: `
version: 1
sites: { a.com: { repo: r } }
environments:
  production:
    nodes: [{ host: 1.2.3.4 }]
`,
			want: []string{"domain is required", "access.admins"},
		},
		"undefined env site": {
			yaml: `
version: 1
access: { admins: [k] }
sites: { a.com: { repo: r } }
environments:
  production:
    nodes: [{ host: 1.2.3.4 }]
    sites: { a.com: { domain: a.com }, b.com: { domain: b.com } }
`,
			want: []string{"b.com: site is not defined"},
		},
		"split needs private ips": {
			yaml: `
version: 1
access: { admins: [k] }
sites: { a.com: { repo: r } }
environments:
  production:
    nodes:
      - { name: w, host: 1.2.3.4, roles: [web] }
      - { name: d, host: 1.2.3.5, roles: [db] }
    sites: { a.com: { domain: a.com } }
`,
			want: []string{`node "w": private_ip is required`, `node "d": private_ip is required`},
		},
		"two web nodes unsupported": {
			yaml: `
version: 1
access: { admins: [k] }
sites: { a.com: { repo: r } }
environments:
  production:
    nodes:
      - { name: w1, host: 1.2.3.4 }
      - { name: w2, host: 1.2.3.5, roles: [web] }
    sites: { a.com: { domain: a.com } }
`,
			want: []string{"exactly one node must have the web role (found 2)"},
		},
		"duplicate domains and bad values": {
			yaml: `
version: 1
access: { admins: [k] }
sites:
  a.com: { repo: r, cache: redis }
  b.com: { repo: r, ssl: maybe }
environments:
  production:
    nodes: [{ host: 1.2.3.4 }]
    php: { version: eight }
    sites:
      a.com: { domain: a.com }
      b.com: { domain: a.com }
`,
			want: []string{`domain "a.com" is also used by a.com`, "cache must be", "ssl must be", "php.version"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c, err := Parse([]byte(tc.yaml))
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Resolve("production")
			if err == nil {
				t.Fatal("expected error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error missing %q:\n%v", w, err)
				}
			}
		})
	}
}

func siteNames(e *Environment) []string {
	var n []string
	for _, s := range e.Sites {
		n = append(n, s.Name)
	}
	return n
}
