package compile_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"flag"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/roots/trellis-cli/pkg/converge/compile"
	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/lock"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
)

var update = flag.Bool("update", false, "rewrite golden files")

func compileProject(t *testing.T, name, env string) *manifest.Manifest {
	t.Helper()
	p, err := project.Open(filepath.Join("..", "testdata", "projects", name))
	if err != nil {
		t.Fatal(err)
	}
	c, err := p.Compile(env, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.Manifest
}

// TestGolden pins the full rendered output for each example project. Run
// `go test ./pkg/converge/compile -update` after intentional changes and
// review the diff: it is exactly what would change on servers.
func TestGolden(t *testing.T) {
	cases := []struct{ project, env string }{
		{"minimal", "production"},
		{"minimal", "development"},
		{"full", "production"},
		{"external-db", "production"},
	}
	for _, tc := range cases {
		{
			m := compileProject(t, tc.project, tc.env)
			name := tc.project + "-" + tc.env + ".golden"
			t.Run(name, func(t *testing.T) {
				var buf bytes.Buffer
				m.Render(&buf, true)
				path := filepath.Join("testdata", name)
				if *update {
					if err := os.MkdirAll("testdata", 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run with -update)", err)
				}
				if buf.String() != string(want) {
					t.Errorf("%s differs from golden; run with -update and review the diff", name)
				}
			})
		}
	}
}

func TestDeterministic(t *testing.T) {
	a := compileProject(t, "full", "production")
	for range 5 {
		b := compileProject(t, "full", "production")
		if a.Hash() != b.Hash() {
			t.Fatal("compiling twice produced different manifests")
		}
		ja, _ := a.Marshal()
		jb, _ := b.Marshal()
		if !bytes.Equal(ja, jb) {
			t.Fatal("JSON encoding differs between runs")
		}
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := compileProject(t, "full", "production")
	b, err := m.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := manifest.Unmarshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.Hash() != m.Hash() {
		t.Error("hash changed after JSON round trip")
	}
}

func get(t *testing.T, m *manifest.Manifest, kind manifest.Kind, name string) *manifest.Resource {
	t.Helper()
	r := m.Get(manifest.ID{Kind: kind, Name: name})
	if r == nil {
		t.Fatalf("%s: missing %s:%s", m.Node, kind, name)
	}
	return r
}

func TestExternalDatabase(t *testing.T) {
	m := compileProject(t, "external-db", "production")
	if m.Get(manifest.ID{Kind: manifest.KindPackage, Name: "mariadb-server"}) != nil {
		t.Error("an external database must not install mariadb-server")
	}
	get(t, m, manifest.KindPackage, "mariadb-client")
	env := get(t, m, manifest.KindFile, "/srv/www/example.com/shared/.env").File.Content
	if !strings.Contains(env, "DB_HOST='staging-db.abc123.us-east-1.rds.amazonaws.com:3306'") {
		t.Errorf(".env:\n%s", env)
	}
	u := get(t, m, manifest.KindDatabaseUser, "staging-db.abc123.us-east-1.rds.amazonaws.com/example_com@%")
	if u.DatabaseUser.Server.Password != "rds-pass" || !u.DatabaseUser.Server.TLS {
		t.Errorf("server = %+v", u.DatabaseUser.Server)
	}
}

// Every supported OS compiles, with its repositories for its release.
func TestEverySupportedOS(t *testing.T) {
	p, err := project.Open(filepath.Join("..", "testdata", "projects", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range config.OSes {
		e, err := p.Config.Resolve("production")
		if err != nil {
			t.Fatal(err)
		}
		e.Server.OS = o.Name
		m, err := compile.Compile(compile.Input{Env: e, Lock: p.Lock, Secrets: loadSecrets(t, p, "production")})
		if err != nil {
			t.Fatalf("%s: %v", o.Name, err)
		}
		for _, repo := range []string{"nginx", "sury-php", "mariadb"} {
			src := get(t, m, manifest.KindFile, "/etc/apt/sources.list.d/trellis-"+repo+".sources").File.Content
			if !strings.Contains(src, "Suites: "+o.Codename+"\n") {
				t.Errorf("%s: %s source:\n%s", o.Name, repo, src)
			}
		}
	}
}

// auto_updates decides what unattended-upgrades installs; trellis.lock
// pins are never upgraded by it.
func TestAutoUpdates(t *testing.T) {
	p, err := project.Open(filepath.Join("..", "testdata", "projects", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	const conf = "/etc/apt/apt.conf.d/52trellis-unattended-upgrades"
	build := func(mode string, pins map[string]string) *manifest.Manifest {
		t.Helper()
		e, err := p.Config.Resolve("production")
		if err != nil {
			t.Fatal(err)
		}
		e.Server.AutoUpdates = mode
		l := *p.Lock
		l.Packages = pins
		m, err := compile.Compile(compile.Input{Env: e, Lock: &l, Secrets: loadSecrets(t, p, "production")})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	all := get(t, build(config.UpdatesAll, map[string]string{"nginx": "1.30.8-1~noble", "libc6": "2.39"}), manifest.KindFile, conf).File.Content
	for _, want := range []string{
		`"origin=Ubuntu,archive=${distro_codename}-updates";`,
		`"origin=nginx,codename=${distro_codename}";`,
		`"origin=deb.sury.org,codename=${distro_codename}";`,
		`"origin=MariaDB,codename=${distro_codename}";`,
		`Dpkg::Options { "--force-confdef"; "--force-confold"; };`,
		`"^libc6$";`, `"^nginx$";`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("all: missing %s in:\n%s", want, all)
		}
	}

	distro := get(t, build(config.UpdatesDistro, nil), manifest.KindFile, conf).File.Content
	if strings.Contains(distro, "Origins-Pattern") || strings.Contains(distro, "Blacklist") {
		t.Errorf("distro adds origins or a blacklist:\n%s", distro)
	}

	// off stops the nightly run; `upgrade` still installs what all would.
	off := build(config.UpdatesOff, nil)
	if c := get(t, off, manifest.KindFile, conf).File.Content; !strings.Contains(c, `"origin=nginx,codename=${distro_codename}";`) {
		t.Errorf("off: missing origins:\n%s", c)
	}
	periodic := get(t, off, manifest.KindFile, "/etc/apt/apt.conf.d/20auto-upgrades").File.Content
	if !strings.Contains(periodic, `Update-Package-Lists "1"`) || !strings.Contains(periodic, `Unattended-Upgrade "0"`) {
		t.Errorf("off: 20auto-upgrades:\n%s", periodic)
	}
}

// overlay adds files to a project directory.
type overlay struct {
	fs.FS
	extra fstest.MapFS
}

func (o overlay) Open(name string) (fs.File, error) {
	if _, ok := o.extra[name]; ok {
		return o.extra.Open(name)
	}
	return o.FS.Open(name)
}

func testCertificate(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"example.com"}, NotAfter: time.Now().AddDate(1, 0, 0)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	k, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: k}))
}

// A site's own certificate is installed as plain files; a key that doesn't
// match fails compilation.
func TestOwnCertificate(t *testing.T) {
	dir := filepath.Join("..", "testdata", "projects", "minimal")
	p, err := project.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cert, key := testCertificate(t)
	_, otherKey := testCertificate(t)
	build := func(key string) (*manifest.Manifest, error) {
		e, err := p.Config.Resolve("production")
		if err != nil {
			t.Fatal(err)
		}
		e.Sites[0].HTTPS = config.HTTPS{Mode: config.HTTPSCustom, Certificate: "file:certs/site.pem", Key: "secret:certs/site_key"}
		sec := loadSecrets(t, p, "production")
		sec["certs"] = map[string]any{"site_key": key}
		return compile.Compile(compile.Input{
			Env: e, Lock: p.Lock, Secrets: sec,
			Project: overlay{os.DirFS(dir), fstest.MapFS{"certs/site.pem": {Data: []byte(cert)}}},
		})
	}

	m, err := build(key)
	if err != nil {
		t.Fatal(err)
	}
	crt := get(t, m, manifest.KindFile, "/etc/nginx/ssl/example.com.crt")
	k := get(t, m, manifest.KindFile, "/etc/nginx/ssl/example.com.key")
	if crt.File.Content != cert || k.File.Content != key || !k.File.Sensitive || k.File.Mode != 0o600 {
		t.Errorf("certificate files: crt %v, key sensitive %v mode %o", crt.File.Content == cert, k.File.Sensitive, k.File.Mode)
	}
	if m.Get(manifest.ID{Kind: manifest.KindCert, Name: "example.com"}) != nil || m.Get(manifest.ID{Kind: manifest.KindPackage, Name: "certbot"}) != nil {
		t.Error("an own certificate still requests one")
	}

	if _, err := build(otherKey); err == nil || !strings.Contains(err.Error(), "don't form a valid pair") {
		t.Errorf("mismatched key: err = %v", err)
	}
}

func TestFullProjectCustomizations(t *testing.T) {
	m := compileProject(t, "full", "production")

	get(t, m, manifest.KindFile, "/etc/nginx/conf.d/realip.conf")
	get(t, m, manifest.KindFile, "/etc/php/8.4/fpm/conf.d/jit.ini")
	get(t, m, manifest.KindFile, "/usr/local/share/fonts/Inter.ttf")
	redirects := get(t, m, manifest.KindFile, "/etc/nginx/includes.d/example.com/redirects.conf").File.Content
	if !strings.Contains(redirects, "https://example.com/new") || !strings.Contains(redirects, "on production") {
		t.Errorf("rendered file:\n%s", redirects)
	}
	// The production overlay.
	get(t, m, manifest.KindFile, "/etc/sysctl.d/90-tuning.conf")

	// Extra packages, deduplicated with core and pinned from lock.
	if v := get(t, m, manifest.KindPackage, "ghostscript").Package.Version; v != "10.02.1~dfsg1-0ubuntu7.4" {
		t.Errorf("ghostscript version = %q", v)
	}

	// The Quadlet recipe: podman, a started unit, and a secret env file.
	get(t, m, manifest.KindPackage, "podman")
	get(t, m, manifest.KindService, "meilisearch.service")
	env := get(t, m, manifest.KindFile, "/etc/trellis/meilisearch.env")
	if env.File.Mode != 0o600 || !env.File.Sensitive || !strings.Contains(env.File.Content, "MEILI_MASTER_KEY=meili-master") {
		t.Errorf("env file = %+v", env.File)
	}
	// The unit's EnvironmentFile= restarts it when the file changes.
	if len(env.Notify) != 1 || env.Notify[0].Unit != "meilisearch.service" {
		t.Errorf("env file notify = %+v", env.Notify)
	}

	fonts := get(t, m, manifest.KindExec, "user:fc-cache-f")
	if fonts.Exec.Inputs == "" {
		t.Error("a command with on: must hash its watched files")
	}
	ts := get(t, m, manifest.KindExec, "user:tailscale-up")
	if ts.Exec.Env["TS_AUTHKEY"] != "tskey-auth-example" || !ts.Sensitive() {
		t.Errorf("command env = %v", ts.Exec.Env)
	}
}

func TestErrorsAreCollected(t *testing.T) {
	c, err := config.Parse([]byte(`
version: 1
access: { admins: [github:missing-from-lock] }
sites:
  a.com: { repo: r, env: { X: secret:nope } }
environments:
  production:
    host: 1.2.3.4
    sites: { a.com: { domain: a.com } }
`))
	if err != nil {
		t.Fatal(err)
	}
	e, err := c.Resolve("production")
	if err != nil {
		t.Fatal(err)
	}
	_, err = compile.Compile(compile.Input{Env: e, Lock: lock.New(), Secrets: secrets.Map{}})
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{
		"github:missing-from-lock is not in trellis.lock",
		`secret "nope" is not defined`,
		"secret sites.a.com.db_password is missing",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
	}
}

func TestOwnershipConflict(t *testing.T) {
	m := manifest.New("n", nil)
	if err := m.Add(manifest.NewRootFile("/etc/x", "a", 0o644)); err != nil {
		t.Fatal(err)
	}
	if err := m.Add(manifest.NewRootFile("/etc/x", "b", 0o644)); err == nil || !strings.Contains(err.Error(), "ownership conflict") {
		t.Fatalf("expected ownership conflict, got %v", err)
	}
}

func loadSecrets(t testing.TB, p *project.Project, env string) secrets.Map {
	t.Helper()
	doc, _, err := p.LoadSecrets(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := doc.Map()
	if err != nil {
		t.Fatal(err)
	}
	return m
}
