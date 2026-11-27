package remote_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/host/hosttest"
	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/release"
	"github.com/roots/trellis-cli/pkg/converge/remote"
)

// session compiles project/env and connects a session to a fake server.
func session(t *testing.T, name, env string, s *remote.Session, f *fakeSSH) (*project.Compiled, *remote.Server) {
	t.Helper()
	p, err := project.Open(filepath.Join("..", "testdata", "projects", name))
	if err != nil {
		t.Fatal(err)
	}
	c, err := p.Compile(env, "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	bin := binary(t)
	s.SSH = &remote.SSH{Exec: f}
	s.FindBinary = func(string) (remote.Binary, error) { return bin, nil }
	srv, err := s.Connect(context.Background(), c.Env, c.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func TestSessionPlanApplyWithExternalDatabase(t *testing.T) {
	ctx := context.Background()
	h := hosttest.New()
	h.Files["/etc/os-release"].Data = []byte("ID=debian\nVERSION_ID=\"13\"\n") // the fixture says os: debian-13
	f := &fakeSSH{host: h, users: map[string]bool{"root": true}, installed: map[string]bool{}}
	var out bytes.Buffer
	s := &remote.Session{Out: &out, User: "root"}
	_, srv := session(t, "external-db", "production", s, f)
	if srv.Target.User != "root" || srv.Target.Host != "198.51.100.7" {
		t.Fatalf("target = %+v", srv.Target)
	}

	if err := s.Plan(ctx, srv); err != nil {
		t.Fatal(err)
	}
	if srv.Plan.Empty() || !strings.Contains(out.String(), "198.51.100.7") {
		t.Fatalf("plan output:\n%s", out.String())
	}
	if err := s.Apply(ctx, srv); err != nil {
		t.Fatal(err)
	}
	if h.Packages["nginx"] == "" || h.Packages["mariadb-server"] != "" || h.Packages["mariadb-client"] == "" {
		t.Error("an external database means no local MariaDB server")
	}
	if !h.Databases["example_com_production"] {
		t.Error("database not created on the external server")
	}

	// Second round: nothing to do.
	if err := s.Plan(ctx, srv); err != nil {
		t.Fatal(err)
	}
	if !srv.Plan.Empty() {
		t.Errorf("not converged:\n%s", out.String())
	}
}

func TestHostOverride(t *testing.T) {
	f := &fakeSSH{host: hosttest.New(), users: map[string]bool{"root": true}, installed: map[string]bool{}}
	s := &remote.Session{Out: &bytes.Buffer{}, User: "root", Host: "10.0.0.9", Port: 2222}
	_, srv := session(t, "minimal", "production", s, f)
	if srv.Target.Host != "10.0.0.9" || srv.Target.Port != 2222 {
		t.Errorf("target = %+v", srv.Target)
	}
	if srv.Name != "203.0.113.10 10.0.0.9" {
		t.Errorf("name = %q", srv.Name)
	}
}

func TestDevelopmentNeedsAHost(t *testing.T) {
	p, _ := project.Open(filepath.Join("..", "testdata", "projects", "minimal"))
	c, err := p.Compile("development", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &remote.Session{SSH: &remote.SSH{}}
	if _, err := s.Connect(context.Background(), c.Env, c.Manifest); err == nil || !strings.Contains(err.Error(), "--lima") {
		t.Fatalf("err = %v", err)
	}
}

func TestSessionDeployAndRollback(t *testing.T) {
	ctx := context.Background()
	h := hosttest.New()
	// web logs in with a deploy key and has no sudo (beyond its PHP-FPM
	// reload rule, which sudo -n true doesn't match).
	f := &fakeSSH{host: h, users: map[string]bool{"web": true}, installed: map[string]bool{}, passwordSudo: true}
	var out bytes.Buffer
	s := &remote.Session{Out: &out, User: "web", RunAs: "web"}
	_, srv := session(t, "minimal", "production", s, f)
	uploadsBefore := f.uploads // the agent binary
	if !f.installed["203.0.113.10/home/web/.local/lib/trellis/agent-"+srv.Binary.SHA256[:16]] {
		t.Errorf("agent not installed in web's home: %v", f.installed)
	}

	art := remote.Artifact{Data: []byte("tarball"), SHA256: "abc123", Path: "/home/web/.cache/trellis/abc123.tar.gz"}
	spec := release.Spec{Site: "example.com", ID: "20261125120000-0123456", Artifact: art.Path, PHPUnit: "php8.4-fpm.service",
		Shared: []release.Link{{Path: ".env", Target: "shared/.env"}}}
	if err := s.Deploy(ctx, srv, art, spec); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if f.uploads != uploadsBefore+1 {
		t.Errorf("artifact uploads = %d, want 1", f.uploads-uploadsBefore)
	}
	if len(h.Commands("tar -xzf /home/web/.cache/trellis/abc123.tar.gz")) != 1 {
		t.Error("release not unpacked")
	}
	if !strings.Contains(out.String(), "✓ 20261125120000-0123456 is live") {
		t.Errorf("output:\n%s", out.String())
	}

	// Same artifact again: not re-uploaded.
	if err := s.Deploy(ctx, srv, art, spec); err != nil {
		t.Fatal(err)
	}
	if f.uploads != uploadsBefore+1 {
		t.Error("an existing artifact was uploaded again")
	}

	h.Outputs = map[string]string{
		"readlink /srv/www/example.com/current": "releases/20261125120000-0123456\n",
		"ls -1 /srv/www/example.com/releases":   "20261120000000-aaaaaaa\n20261125120000-0123456\n",
	}
	out.Reset()
	if err := s.Rollback(ctx, srv, "example.com", "php8.4-fpm.service"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "rolled back 20261125120000-0123456 → 20261120000000-aaaaaaa") {
		t.Errorf("output:\n%s", out.String())
	}
	for _, u := range f.agentUsers {
		if u != "web" {
			t.Errorf("a deploy ran the agent as %s", u)
		}
	}
}

// A user with passwordless sudo (a Lima VM's) deploys through sudo -u web,
// with the agent apply installed.
func TestDeployThroughSudo(t *testing.T) {
	ctx := context.Background()
	h := hosttest.New()
	f := &fakeSSH{host: h, users: map[string]bool{"lima": true}, installed: map[string]bool{}}
	bin := binary(t)
	f.installed["203.0.113.10"+bin.RemotePath()] = true
	var out bytes.Buffer
	s := &remote.Session{Out: &out, User: "lima", RunAs: "web"}
	_, srv := session(t, "minimal", "production", s, f)
	if f.uploads != 0 {
		t.Errorf("uploaded the agent although apply installed it")
	}
	art := remote.Artifact{Data: []byte("tarball"), SHA256: "abc123", Path: "/home/web/.cache/trellis/abc123.tar.gz"}
	spec := release.Spec{Site: "example.com", ID: "20261125120000-0123456", Artifact: art.Path, PHPUnit: "php8.4-fpm.service"}
	if err := s.Deploy(ctx, srv, art, spec); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(f.agentUsers) != 1 || f.agentUsers[0] != "web" {
		t.Errorf("agent ran as %v", f.agentUsers)
	}
}
