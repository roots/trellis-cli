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
	if e.Host != "203.0.113.10" || e.Port != 22 || !e.LocalDB() || e.ServerName() != "203.0.113.10" {
		t.Fatalf("server = %s:%d local db %v", e.Host, e.Port, e.LocalDB())
	}
	s := e.Sites[0]
	if s.HTTPS.Mode != HTTPSLetsEncrypt || s.Branch != "main" || s.Cache != CacheOff {
		t.Errorf("site defaults not applied: %+v", s)
	}
	if diff := cmp.Diff([]string{"example.com", "www.example.com"}, s.Domains()); diff != "" {
		t.Error(diff)
	}
	if s.DBName() != "example_com_production" || s.DBUser() != "example_com" {
		t.Errorf("db name = %q", s.DBName())
	}
}

func TestDevelopmentNeedsNoHostAndIsSelfSigned(t *testing.T) {
	e := resolve(t, "minimal", "development")
	if e.Host != "" || e.ServerName() != DevelopmentEnv {
		t.Fatalf("host = %q, name = %q", e.Host, e.ServerName())
	}
	if e.Sites[0].HTTPS.Mode != HTTPSSelfSigned {
		t.Errorf("https = %q", e.Sites[0].HTTPS.Mode)
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

func TestExternalDatabase(t *testing.T) {
	ext := resolve(t, "external-db", "production")
	if ext.Database == nil || ext.Database.Port != 3306 || ext.LocalDB() {
		t.Fatalf("database = %+v", ext.Database)
	}
}

func TestNodesAreNotSupported(t *testing.T) {
	_, err := Parse([]byte("version: 1\nenvironments:\n  production:\n    nodes: [{ host: 1.2.3.4 }]\n"))
	if err == nil || !strings.Contains(err.Error(), "nodes") {
		t.Fatalf("nodes must be rejected (one host per environment), got %v", err)
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
    host: 1.2.3.4
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
    host: 1.2.3.4
    sites: { a.com: { domain: a.com }, b.com: { domain: b.com } }
`,
			want: []string{"b.com: site is not defined"},
		},
		"host is required outside development": {
			yaml: `
version: 1
access: { admins: [k] }
sites: { a.com: { repo: r } }
environments:
  production:
    sites: { a.com: { domain: a.com } }
`,
			want: []string{"environments.production: host is required"},
		},
		"unsafe site name and unknown time zone": {
			yaml: `
version: 1
access: { admins: [k] }
sites: { "../../etc/evil": { repo: r, domain: a.com } }
environments:
  production:
    host: 1.2.3.4
    timezone: Mars/Olympus
`,
			want: []string{"site name must be lowercase", `timezone "Mars/Olympus" is not a known time zone`},
		},
		"unsupported os": {
			yaml: `
version: 1
access: { admins: [k] }
sites: { a.com: { repo: r } }
environments:
  production:
    host: 1.2.3.4
    os: debian-12
    sites: { a.com: { domain: a.com } }
`,
			want: []string{`os "debian-12" is not supported (supported: ubuntu-24.04, ubuntu-26.04, debian-13)`},
		},
		"duplicate domains and bad values": {
			yaml: `
version: 1
access: { admins: [k] }
sites:
  a.com: { repo: r, cache: redis }
  b.com: { repo: r, https: { certificate: certs/b.pem, key: plain-key } }
environments:
  production:
    host: 1.2.3.4
    php: { version: eight }
    sites:
      a.com: { domain: a.com }
      b.com: { domain: a.com }
`,
			want: []string{`domain "a.com" is also used by a.com`, "cache must be", "https.certificate must be file:", "https.key must be secret:", "php.version"},
		},
		"database users cut to the same 32 characters": {
			yaml: `
version: 1
access: { admins: [k] }
sites:
  a-very-long-site-name-for-a-client.com: { repo: r, domain: a.com }
  a-very-long-site-name-for-a-client.org: { repo: r, domain: b.com }
environments:
  production: { host: 1.2.3.4 }
`,
			want: []string{`database user "a_very_long_site_name_for_a_clie" collides with a-very-long-site-name-for-a-client.com; set db.user`},
		},
		"per-site fpm": {
			yaml: `
version: 1
access: { admins: [k] }
sites:
  a.com: { repo: r, domain: a.com, php: { fpm: { pm: lazy } } }
environments:
  production: { host: 1.2.3.4 }
`,
			want: []string{"sites.a.com: php.fpm.pm must be"},
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

// https: accepts true, false, self-signed or the site's own certificate.
func TestHTTPSForms(t *testing.T) {
	for in, want := range map[string]HTTPS{
		"":                   {Mode: HTTPSLetsEncrypt},
		"https: true":        {Mode: HTTPSLetsEncrypt},
		"https: false":       {Mode: HTTPSOff},
		"https: self-signed": {Mode: HTTPSSelfSigned},
		"https: { certificate: file:certs/a.pem, key: secret:certs/a_key }": {Mode: HTTPSCustom, Certificate: "file:certs/a.pem", Key: "secret:certs/a_key"},
	} {
		c, err := Parse([]byte("version: 1\naccess: { admins: [k] }\nsites: { a.com: { repo: r, " + strings.TrimSpace(in+" ") + " } }\nenvironments: { production: { host: 1.2.3.4, sites: { a.com: { domain: a.com } } } }\n"))
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		e, err := c.Resolve("production")
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if got := e.Sites[0].HTTPS; got != want {
			t.Errorf("%q: got %+v, want %+v", in, got, want)
		}
	}
	for _, bad := range []string{"https: letsencrypt", "https: none"} {
		if _, err := Parse([]byte("version: 1\nsites: { a.com: { repo: r, " + bad + " } }\n")); err == nil || !strings.Contains(err.Error(), "https must be true, false, self-signed") {
			t.Errorf("%q: err = %v", bad, err)
		}
	}
}

func siteNames(e *Environment) []string {
	var n []string
	for _, s := range e.Sites {
		n = append(n, s.Name)
	}
	return n
}

func TestSiteDeploySettings(t *testing.T) {
	c, err := Parse([]byte(`version: 1
sites:
  a.com: { repo: git@github.com:x/a.git, domain: a.com }
  b.com:
    repo: git@github.com:x/b.git
    domain: b.com
    deploy: { keep: 2, update_db: false, exclude: ["*.map"] }
access: { admins: ["ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKey me"] }
environments:
  production:
    host: 203.0.113.10
`))
	if err != nil {
		t.Fatal(err)
	}
	e, err := c.Resolve("production")
	if err != nil {
		t.Fatal(err)
	}
	a, b := e.Sites[0], e.Sites[1]
	if a.Deploy.Keep != 5 || !a.Deploy.UpdatesDB() {
		t.Errorf("defaults: %+v", a.Deploy)
	}
	if b.Deploy.Keep != 2 || b.Deploy.UpdatesDB() || len(b.Deploy.Exclude) != 1 {
		t.Errorf("b.com: %+v", b.Deploy)
	}

	for _, bad := range []string{`deploy: { keep: -1 }`, `deploy: { exclude: ["[oops"] }`, `deploy: { bogus: 1 }`} {
		c, err := Parse([]byte("version: 1\nsites:\n  a.com: { repo: git@github.com:x/a.git, domain: a.com, " + strings.TrimSuffix(strings.TrimPrefix(bad, "deploy: "), "") + " }\naccess: { admins: [\"ssh-ed25519 AAAA me\"] }\nenvironments:\n  production:\n    host: 203.0.113.10\n"))
		if err == nil {
			_, err = c.Resolve("production")
		}
		if err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

// A site's php.fpm overrides the server's field by field, and wp_login
// bans are on unless turned off.
func TestPerSiteFPMAndFail2ban(t *testing.T) {
	c, err := Parse([]byte(`
version: 1
access: { admins: [k] }
defaults:
  php: { fpm: { max_children: 20, max_requests: 300 } }
sites:
  a.com: { repo: r, domain: a.com, php: { fpm: { max_children: 4 } } }
  b.com: { repo: r, domain: b.com }
environments:
  production: { host: 1.2.3.4 }
  staging:
    host: 1.2.3.5
    security: { fail2ban: { wp_login: false } }
`))
	if err != nil {
		t.Fatal(err)
	}
	e, err := c.Resolve("production")
	if err != nil {
		t.Fatal(err)
	}
	a, b := e.Sites[0].PHP.FPM, e.Sites[1].PHP.FPM
	if a.MaxChildren != 4 || a.MaxRequests != 300 || a.PM != "dynamic" || b.MaxChildren != 20 {
		t.Errorf("fpm: a %+v, b %+v", a, b)
	}
	if !*e.Server.Security.Fail2ban.WPLogin {
		t.Error("wp_login bans are off by default")
	}
	st, err := c.Resolve("staging")
	if err != nil || *st.Server.Security.Fail2ban.WPLogin {
		t.Errorf("wp_login: false ignored: %v", err)
	}
}
