package engine_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/engine"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
)

// secretValues are the fixtures' secrets long enough to search for.
var secretValues = []string{
	"postmark-pass", "sk_live_example", "meili-master", "example-db-pass",
	"shop-db-pass", "split-db-pass", "rds-pass",
}

func assertNoSecrets(t *testing.T, where string, data []byte) {
	t.Helper()
	for _, s := range secretValues {
		if strings.Contains(string(data), s) {
			t.Errorf("%s contains secret %q", where, s)
		}
	}
}

func TestGenerationsAndPlansNeverContainSecrets(t *testing.T) {
	for _, tc := range []struct{ proj, env string }{
		{"full", "production"},
		{"split", "staging"}, // external DB: admin password in every DB resource
	} {
		t.Run(tc.proj+"/"+tc.env, func(t *testing.T) {
			h := newFake()
			m := compileEnv(t, tc.proj, tc.env, nil)[0]

			p, _ := plan(t, h, m)
			red := p.Redacted()
			b, _ := json.Marshal(red)
			assertNoSecrets(t, "redacted plan", b)
			if red.Hash() != p.Hash() {
				t.Error("redacting a plan changed its hash; approval would break")
			}

			converge(t, h, m)
			for path, f := range h.Files {
				if strings.HasPrefix(path, "/var/lib/trellis/") {
					assertNoSecrets(t, path, f.Data)
				}
			}
			// History is redacted, yet nothing looks changed.
			if p, _ := plan(t, h, m); !p.Empty() {
				t.Fatalf("plan after apply is not empty:\n%s", describe(p))
			}
		})
	}
}

func TestRedactionPreservesDigests(t *testing.T) {
	srv := &manifest.DBServer{Host: "db", User: "admin", Password: "admin-pass"}
	for _, r := range []manifest.Resource{
		manifest.NewDatabaseUser("u", "%", "user-pass", []string{"d"}, srv),
		manifest.NewDatabase("d", srv),
		manifest.NewRootFile("/etc/x", "token=abc", 0o600).AsSensitive(),
	} {
		red := r.Redacted()
		if red.Digest() != r.Digest() {
			t.Errorf("%s: digest changed by redaction", r.ID())
		}
		if again := red.Redacted(); again.Digest() != red.Digest() {
			t.Errorf("%s: redaction is not idempotent", r.ID())
		}
		b, _ := json.Marshal(red)
		for _, s := range []string{"admin-pass", "user-pass", "token=abc"} {
			if strings.Contains(string(b), s) {
				t.Errorf("%s: redacted form contains %q", r.ID(), s)
			}
		}
	}
	if srv.Password != "admin-pass" {
		t.Error("redaction mutated the original")
	}
	a := manifest.NewDatabaseUser("u", "%", "one", nil, nil)
	b := manifest.NewDatabaseUser("u", "%", "two", nil, nil)
	if a.Digest() == b.Digest() {
		t.Error("a changed password must change the digest")
	}
}

func dbManifest(adminPassword string, withUser bool) *manifest.Manifest {
	srv := &manifest.DBServer{Host: "db.internal", User: "admin", Password: adminPassword}
	m := manifest.New("web", []string{"web"})
	_ = m.Add(manifest.NewDatabase("app", srv))
	if withUser {
		_ = m.Add(manifest.NewDatabaseUser("app", "%", "app-pass", []string{"app"}, srv))
	}
	m.Finalize()
	return m
}

func TestPruningRemoteDBUserUsesCurrentCredentials(t *testing.T) {
	h := newFake()
	converge(t, h, dbManifest("old-admin", true))

	// The admin password rotated and the user is no longer wanted: the
	// drop must use the current credentials, not the (redacted) old ones.
	m := dbManifest("new-admin", false)
	p, obs := plan(t, h, m)
	var prune *engine.Action
	for i := range p.Actions {
		if p.Actions[i].Op == engine.OpPrune {
			prune = &p.Actions[i]
		}
	}
	if prune == nil || prune.Resource.DatabaseUser.Server.Password != "new-admin" {
		t.Fatalf("plan:\n%s", describe(p))
	}
	if _, err := engine.Apply(context.Background(), h, m, p, obs.Previous, engine.Options{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := h.DBUsers["app@%"]; ok {
		t.Error("user not dropped")
	}
}

func TestPruningRemoteDBUserWithoutCredentialsWarns(t *testing.T) {
	h := newFake()
	converge(t, h, dbManifest("admin", true))

	m := manifest.New("web", []string{"web"})
	m.Finalize()
	p, _ := plan(t, h, m)
	for _, a := range p.Actions {
		if a.Resource.Kind == manifest.KindDatabaseUser {
			t.Fatalf("unexpected action %s %s", a.Op, a.ID())
		}
	}
	if !strings.Contains(strings.Join(p.Warnings, "\n"), "has no credentials in the manifest") {
		t.Errorf("warnings = %q", p.Warnings)
	}
}
