package deploy_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/deploy"
)

func rev(t *testing.T, dir, ref string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "rev-parse", ref).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

// origin (bare) and a clone of it, both with one commit on main.
func clone(t *testing.T) (origin, local string) {
	t.Helper()
	dir := t.TempDir()
	origin, seed, local := filepath.Join(dir, "origin.git"), filepath.Join(dir, "seed"), filepath.Join(dir, "local")
	git(t, dir, "init", "-q", "--bare", "-b", "main", origin)
	git(t, dir, "init", "-q", "-b", "main", seed)
	write(t, filepath.Join(seed, "a"), "1")
	git(t, seed, "add", ".")
	git(t, seed, "commit", "-qm", "one")
	git(t, seed, "push", "-q", origin, "main")
	git(t, dir, "clone", "-q", origin, local)
	return origin, local
}

func commit(t *testing.T, dir, msg string) {
	t.Helper()
	write(t, filepath.Join(dir, msg), msg)
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", msg)
}

func TestResolveDeploysTheRemoteBranch(t *testing.T) {
	ctx := context.Background()
	origin, local := clone(t)

	// Someone else pushed; the local main is behind.
	other := filepath.Join(t.TempDir(), "other")
	git(t, ".", "clone", "-q", origin, other)
	commit(t, other, "two")
	git(t, other, "push", "-q", "origin", "main")

	src, err := deploy.Resolve(ctx, local, "", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if src.Commit != rev(t, other, "HEAD") || src.From != "main on origin" || len(src.Notes) != 0 {
		t.Errorf("behind: %+v", src)
	}

	// An unpushed local commit isn't deployed, and the user is told.
	git(t, local, "pull", "-q", "--ff-only")
	commit(t, local, "three")
	src, err = deploy.Resolve(ctx, local, "", "main", false)
	if err != nil {
		t.Fatal(err)
	}
	if src.Commit != rev(t, other, "HEAD") || len(src.Notes) != 1 || !strings.Contains(src.Notes[0], "1 commit(s) not on origin") {
		t.Errorf("ahead: %+v", src)
	}

	// --ref deploys exactly what the clone has.
	src, err = deploy.Resolve(ctx, local, "main", "main", false)
	if err != nil || src.Commit != rev(t, local, "main") {
		t.Errorf("--ref: %+v %v", src, err)
	}
}

func TestResolveInCIUsesTheCheckout(t *testing.T) {
	_, local := clone(t)
	commit(t, local, "two") // e.g. a detached checkout of the triggering commit
	src, err := deploy.Resolve(context.Background(), local, "", "main", true)
	if err != nil || src.Commit != rev(t, local, "HEAD") {
		t.Errorf("CI: %+v %v", src, err)
	}
}

func TestResolveWithoutARemote(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	commit(t, dir, "one")
	src, err := deploy.Resolve(context.Background(), dir, "", "main", false)
	if err != nil || src.Commit != rev(t, dir, "main") || len(src.Notes) != 1 {
		t.Errorf("no remote: %+v %v", src, err)
	}
	if _, err := deploy.Resolve(context.Background(), dir, "", "develop", false); err == nil {
		t.Error("deployed a branch that doesn't exist")
	}
}

func TestResolveFetchFailureSuggestsRef(t *testing.T) {
	_, local := clone(t)
	git(t, local, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "gone.git"))
	_, err := deploy.Resolve(context.Background(), local, "", "main", false)
	if err == nil || !strings.Contains(err.Error(), "--ref main") {
		t.Errorf("err = %v", err)
	}
}
