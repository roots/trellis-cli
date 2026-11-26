package deploy_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/deploy"
	"github.com/roots/trellis-cli/pkg/converge/release"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
}

func entries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		out[h.Name] = string(b)
	}
}

func TestBuildPacksTheCommitNotTheWorkingTree(t *testing.T) {
	repo, bin := t.TempDir(), t.TempDir()
	write(t, filepath.Join(repo, "site/composer.json"), `{}`)
	write(t, filepath.Join(repo, "site/web/app/themes/t/style.css"), "committed")
	write(t, filepath.Join(repo, "README.md"), "outside the subtree")
	git(t, repo, "init", "-q", "-b", "main")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "init")
	// Uncommitted changes must not be deployed.
	write(t, filepath.Join(repo, "site/web/app/themes/t/style.css"), "dirty")
	write(t, filepath.Join(repo, "site/untracked.php"), "untracked")

	composer := filepath.Join(bin, "composer")
	write(t, composer, "#!/bin/sh\nmkdir -p vendor && echo '<?php' > vendor/autoload.php\n")
	hook := filepath.Join(bin, "build.sh")
	write(t, hook, "mkdir -p web/app/themes/t/dist web/app/themes/t/node_modules/x\necho built > web/app/themes/t/dist/app.js\necho junk > web/app/themes/t/node_modules/x/i.js\n")

	art, err := deploy.Build(context.Background(), deploy.BuildOptions{
		Repo: repo, Ref: "main", Subtree: "site", Hook: hook, Composer: []string{composer},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := entries(t, art.Data)
	if got["web/app/themes/t/style.css"] != "committed" {
		t.Errorf("style.css = %q, want the committed content", got["web/app/themes/t/style.css"])
	}
	for _, name := range []string{"untracked.php", "README.md", "web/app/themes/t/node_modules/x/i.js"} {
		if _, ok := got[name]; ok {
			t.Errorf("%s must not be packed", name)
		}
	}
	for _, name := range []string{"vendor/autoload.php", "web/app/themes/t/dist/app.js", "composer.json"} {
		if _, ok := got[name]; !ok {
			t.Errorf("%s missing; have %v", name, keys(got))
		}
	}
	if len(art.Commit) != 40 || len(art.SHA256) != 64 {
		t.Errorf("artifact = %s %s", art.Commit, art.SHA256)
	}

	if _, err := deploy.Build(context.Background(), deploy.BuildOptions{Repo: repo, Ref: "nope", Composer: []string{composer}}); err == nil || !strings.Contains(err.Error(), "no commit") {
		t.Errorf("unknown ref: %v", err)
	}
	if _, err := deploy.Build(context.Background(), deploy.BuildOptions{Repo: repo, Ref: "main", Composer: []string{composer}}); err == nil || !strings.Contains(err.Error(), "composer.json") {
		t.Errorf("wrong subtree: %v", err)
	}
}

func keys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestSpec(t *testing.T) {
	env := &config.Environment{Name: "production"}
	env.Server.PHP.Version = "8.4"
	site := config.Site{Name: "example.com", Writable: []string{"web/app/uploads", "web/app/cache/"}}
	art := deploy.Artifact{SHA256: "ab12", Commit: "0123456789abcdef"}
	s := deploy.Spec(env, site, art, release.Hooks{After: "true"}, time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC))
	if s.ID != "20261125120000-0123456" || s.Artifact != "/var/cache/trellis/ab12.tar.gz" || s.PHPUnit != "php8.4-fpm.service" {
		t.Errorf("spec = %+v", s)
	}
	want := []release.Link{
		{Path: ".env", Target: "shared/.env"},
		{Path: "web/app/uploads", Target: "shared/uploads"},
		{Path: "web/app/cache", Target: "shared/cache"},
	}
	if !slices.Equal(s.Shared, want) {
		t.Errorf("shared = %v", s.Shared)
	}
	if err := s.Validate(); err != nil {
		t.Error(err)
	}
}
