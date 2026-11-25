package remote_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/host/hosttest"
	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/remote"
)

func TestSessionSplitEnvironment(t *testing.T) {
	ctx := context.Background()
	p, err := project.Open(filepath.Join("..", "testdata", "projects", "split"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := p.Compile("production", "test", nil)
	if err != nil {
		t.Fatal(err)
	}

	web, db := hosttest.New(), hosttest.New()
	for _, h := range []*hosttest.Fake{web, db} {
		h.GPG["content of https://packages.sury.org/php/apt.gpg"] = "15058500A0235D97F5D10063B188E2B695BD4743"
	}
	f := &fakeSSH{
		hosts:     map[string]*hosttest.Fake{"203.0.113.10": web, "203.0.113.11": db},
		users:     map[string]bool{"root": true},
		installed: map[string]bool{},
	}
	bin := binary(t)
	var out bytes.Buffer
	s := &remote.Session{
		SSH:        &remote.SSH{Exec: f},
		FindBinary: func(string) (remote.Binary, error) { return bin, nil },
		Out:        &out,
		Users:      []string{"admin", "root"},
	}

	runs, err := s.Prepare(ctx, c.Env, c.Manifests)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].Node.Name != "db1" || runs[1].Node.Name != "web1" {
		t.Fatalf("db node must come first, got %s, %s", runs[0].Node.Name, runs[1].Node.Name)
	}
	if err := s.Plan(ctx, runs); err != nil {
		t.Fatal(err)
	}
	if remote.Empty(runs) {
		t.Fatal("expected changes")
	}
	if !strings.Contains(out.String(), "db1 203.0.113.11") || !strings.Contains(out.String(), "+ package       mariadb-server") {
		t.Errorf("plan output:\n%s", out.String())
	}

	out.Reset()
	if err := s.Apply(ctx, runs); err != nil {
		t.Fatal(err)
	}
	if !db.Databases["example_com"] || db.Packages["nginx"] != "" {
		t.Error("db node state wrong")
	}
	if web.Packages["nginx"] == "" || web.Packages["mariadb-server"] != "" {
		t.Error("web node state wrong")
	}
	if strings.Index(out.String(), "db1") > strings.Index(out.String(), "web1") {
		t.Error("db node must be applied first")
	}

	// Second round: nothing to do on either node.
	if err := s.Plan(ctx, runs); err != nil {
		t.Fatal(err)
	}
	if !remote.Empty(runs) {
		t.Errorf("not converged:\n%s", out.String())
	}
}

func TestHostOverrideOnlyForSingleNode(t *testing.T) {
	p, _ := project.Open(filepath.Join("..", "testdata", "projects", "split"))
	c, err := p.Compile("production", "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &remote.Session{SSH: &remote.SSH{}, Host: "10.0.0.9"}
	if _, err := s.Prepare(context.Background(), c.Env, c.Manifests); err == nil {
		t.Fatal("expected error for --host with two nodes")
	}
}
