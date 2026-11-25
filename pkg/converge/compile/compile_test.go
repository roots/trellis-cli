package compile_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/roots/trellis-cli/pkg/converge/compile"
	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/lock"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
)

var update = flag.Bool("update", false, "rewrite golden files")

func compileProject(t *testing.T, name, env string) []*manifest.Manifest {
	t.Helper()
	p, err := project.Open(filepath.Join("..", "testdata", "projects", name))
	if err != nil {
		t.Fatal(err)
	}
	c, err := p.Compile(env, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	return c.Manifests
}

// TestGolden pins the full rendered output for each example project. Run
// `go test ./pkg/converge/compile -update` after intentional changes and
// review the diff: it is exactly what would change on servers.
func TestGolden(t *testing.T) {
	cases := []struct{ project, env string }{
		{"minimal", "production"},
		{"minimal", "development"},
		{"full", "production"},
		{"split", "production"},
		{"split", "staging"},
	}
	for _, tc := range cases {
		for _, m := range compileProject(t, tc.project, tc.env) {
			name := tc.project + "-" + tc.env + "-" + m.Node + ".golden"
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
		if a[0].Hash() != b[0].Hash() {
			t.Fatal("compiling twice produced different manifests")
		}
		ja, _ := a[0].Marshal()
		jb, _ := b[0].Marshal()
		if !bytes.Equal(ja, jb) {
			t.Fatal("JSON encoding differs between runs")
		}
	}
}

func TestManifestRoundTrip(t *testing.T) {
	m := compileProject(t, "full", "production")[0]
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

func TestSplitTopology(t *testing.T) {
	ms := compileProject(t, "split", "production")
	if len(ms) != 2 {
		t.Fatalf("want 2 manifests, got %d", len(ms))
	}
	web, db := ms[0], ms[1]

	env := get(t, web, manifest.KindFile, "/srv/www/example.com/shared/.env").File.Content
	if !strings.Contains(env, "DB_HOST='10.0.0.5'") {
		t.Errorf("web .env should point at db private ip:\n%s", env)
	}
	if web.Get(manifest.ID{Kind: manifest.KindPackage, Name: "mariadb-server"}) != nil {
		t.Error("web node must not install mariadb-server")
	}
	get(t, web, manifest.KindPackage, "mariadb-client")

	cnf := get(t, db, manifest.KindFile, "/etc/mysql/mariadb.conf.d/90-trellis.cnf").File.Content
	for _, want := range []string{"bind-address = 10.0.0.5", "innodb_buffer_pool_size = 4G"} {
		if !strings.Contains(cnf, want) {
			t.Errorf("db cnf missing %q:\n%s", want, cnf)
		}
	}
	fw := get(t, db, manifest.KindFile, "/etc/nftables.conf").File.Content
	if !strings.Contains(fw, "ip saddr 10.0.0.4 tcp dport 3306 accept") {
		t.Errorf("db firewall should allow web node:\n%s", fw)
	}
	if strings.Contains(fw, "dport { 80, 443 }") {
		t.Error("db node must not open http")
	}
	get(t, db, manifest.KindDatabaseUser, "example_com@10.0.0.4")
	if db.Get(manifest.ID{Kind: manifest.KindPackage, Name: "nginx"}) != nil {
		t.Error("db node must not install nginx")
	}
}

func TestExternalDatabase(t *testing.T) {
	m := compileProject(t, "split", "staging")[0]
	env := get(t, m, manifest.KindFile, "/srv/www/example.com/shared/.env").File.Content
	if !strings.Contains(env, "DB_HOST='staging-db.abc123.us-east-1.rds.amazonaws.com:3306'") {
		t.Errorf(".env:\n%s", env)
	}
	u := get(t, m, manifest.KindDatabaseUser, "staging-db.abc123.us-east-1.rds.amazonaws.com/example_com@%")
	if u.DatabaseUser.Server.Password != "rds-pass" || !u.DatabaseUser.Server.TLS {
		t.Errorf("server = %+v", u.DatabaseUser.Server)
	}
}

func TestExtensionLadder(t *testing.T) {
	m := compileProject(t, "full", "production")[0]

	// Rung 2: drop-ins, including a rendered .tmpl with site context.
	get(t, m, manifest.KindFile, "/etc/nginx/conf.d/realip.conf")
	get(t, m, manifest.KindFile, "/etc/php/8.4/fpm/conf.d/jit.ini")
	redirects := get(t, m, manifest.KindFile, "/etc/nginx/includes.d/example.com/redirects.conf").File.Content
	if !strings.Contains(redirects, "https://example.com/new") || !strings.Contains(redirects, "on production") {
		t.Errorf("rendered drop-in:\n%s", redirects)
	}
	// Exec inputs under server/ are not installed as drop-ins.
	for _, r := range m.Resources {
		if strings.Contains(r.Name, "fonts") && r.Kind == manifest.KindFile {
			t.Errorf("exec input installed as file: %s", r.Name)
		}
	}

	// Rung 3: extra packages, deduplicated with core and pinned from lock.
	if v := get(t, m, manifest.KindPackage, "ghostscript").Package.Version; v != "10.02.1~dfsg1-0ubuntu7.4" {
		t.Errorf("ghostscript version = %q", v)
	}
	get(t, m, manifest.KindPackage, "php8.4-imagick")

	// Rung 4: Quadlet service pinned by digest, secrets in a 0600 env file.
	q := get(t, m, manifest.KindFile, "/etc/containers/systemd/meilisearch.container").File.Content
	if !strings.Contains(q, "Image=getmeili/meilisearch:v1.11@sha256:4f3c") {
		t.Errorf("quadlet:\n%s", q)
	}
	envFile := get(t, m, manifest.KindFile, "/etc/trellis/services/meilisearch.env")
	if envFile.File.Mode != 0o600 || !envFile.File.Sensitive || !strings.Contains(envFile.File.Content, "MEILI_MASTER_KEY=meili-master") {
		t.Errorf("env file = %+v", envFile.File)
	}

	// Escape hatch.
	x := get(t, m, manifest.KindExec, "user:rebuild-font-cache")
	if x.Exec.Inputs == "" {
		t.Error("exec inputs hash is empty")
	}
}

func TestExecInputsHashTracksFiles(t *testing.T) {
	compileWith := func(font string) string {
		p, err := project.Open(filepath.Join("..", "testdata", "projects", "full"))
		if err != nil {
			t.Fatal(err)
		}
		e, err := p.Config.Resolve("production")
		if err != nil {
			t.Fatal(err)
		}
		sec := loadSecrets(t, p, "production")
		fsys := fstest.MapFS{
			"keys/alice.pub":      {Data: []byte("ssh-ed25519 AAAA alice")},
			"server/fonts/a.ttf":  {Data: []byte(font)},
			"server/php/conf.d/x": nil,
		}
		delete(fsys, "server/php/conf.d/x")
		ms, err := compile.Compile(compile.Input{Env: e, Lock: p.Lock, Secrets: sec, Project: fsys})
		if err != nil {
			t.Fatal(err)
		}
		return ms[0].Get(manifest.ID{Kind: manifest.KindExec, Name: "user:rebuild-font-cache"}).Exec.Inputs
	}
	if compileWith("v1") == compileWith("v2") {
		t.Error("changing an input file must change the exec inputs hash")
	}
	if a, b := compileWith("v1"), compileWith("v1"); a != b {
		t.Error("inputs hash must be stable")
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
    nodes: [{ host: 1.2.3.4 }]
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

func TestDropInConflictsWithCore(t *testing.T) {
	p, err := project.Open(filepath.Join("..", "testdata", "projects", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	e, _ := p.Config.Resolve("production")
	sec := loadSecrets(t, p, "production")
	fsys := fstest.MapFS{
		// Collides with Trellis's own php ini.
		"server/php/conf.d/90-trellis.ini": {Data: []byte("x=1")},
		"server/nginx/typo/oops.conf":      {Data: []byte("")},
	}
	_, err = compile.Compile(compile.Input{Env: e, Lock: p.Lock, Secrets: sec, Project: fsys})
	if err == nil {
		t.Fatal("expected errors")
	}
	for _, want := range []string{"ownership conflict: file:/etc/php/8.4/fpm/conf.d/90-trellis.ini", "server/nginx/typo/oops.conf: unrecognized drop-in"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in:\n%v", want, err)
		}
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
