// Package deploy builds a release locally and describes it for the agent.
//
// Build archives an exact commit from the site's local git repository
// (never the working tree), installs Composer dependencies with the user's
// own Composer and credentials, runs the build hook (e.g. theme assets), and
// packs the result. The server only unpacks and switches (see release), so
// it needs no git, Composer or repository credentials.
package deploy

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/release"
)

// HooksDir holds deploy hooks, relative to the project root.
const HooksDir = "deploy-hooks"

// ArtifactDir is where artifacts are uploaded on servers.
const ArtifactDir = "/var/cache/trellis"

// Artifact is a packed release.
type Artifact struct {
	Data   []byte // .tar.gz
	SHA256 string
	Commit string
	Files  int
}

// RemotePath is where the artifact is uploaded.
func (a Artifact) RemotePath() string { return ArtifactDir + "/" + a.SHA256 + ".tar.gz" }

// BuildOptions configures a build.
type BuildOptions struct {
	Repo    string // local git repository (the site's local_path)
	Ref     string // branch, tag or commit
	Subtree string // path of the site within the repo (site.path)
	Hook    string // build hook script, if any
	Env     []string
	// Composer is the composer command (default: composer).
	Composer []string
	// Out receives build output (Composer, hook).
	Out io.Writer
}

// excluded directories are never packed.
var excluded = map[string]bool{".git": true, "node_modules": true}

// Build produces an artifact for o.Ref.
func Build(ctx context.Context, o BuildOptions) (Artifact, error) {
	if o.Out == nil {
		o.Out = io.Discard
	}
	if len(o.Composer) == 0 {
		o.Composer = []string{"composer"}
	}
	commit, err := output(ctx, o.Repo, "git", "rev-parse", "--verify", "--quiet", o.Ref+"^{commit}")
	if err != nil {
		return Artifact{}, fmt.Errorf("%s: no commit %q (is it fetched?)", o.Repo, o.Ref)
	}
	dir, err := os.MkdirTemp("", "trellis-build-")
	if err != nil {
		return Artifact{}, err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	tree := commit
	if sub := strings.Trim(o.Subtree, "/"); sub != "" {
		tree += ":" + sub
	}
	archive := exec.CommandContext(ctx, "git", "archive", "--format=tar", tree)
	archive.Dir = o.Repo
	var tarball, stderr bytes.Buffer
	archive.Stdout, archive.Stderr = &tarball, &stderr
	if err := archive.Run(); err != nil {
		return Artifact{}, fmt.Errorf("git archive %s: %w: %s", tree, err, strings.TrimSpace(stderr.String()))
	}
	if err := extract(&tarball, dir); err != nil {
		return Artifact{}, err
	}

	if _, err := os.Stat(filepath.Join(dir, "composer.json")); err != nil {
		return Artifact{}, fmt.Errorf("no composer.json in %s at %s (set the site's path to the Bedrock directory)", o.Repo, short(commit))
	}
	// Trellis's flags. Platform checks are skipped here (this machine's PHP
	// isn't the server's); the server runs Composer's platform check before
	// switching.
	composer := slices.Concat(o.Composer, []string{"install", "--no-dev", "--no-interaction", "--no-progress", "--prefer-dist",
		"--optimize-autoloader", "--classmap-authoritative", "--no-scripts", "--ignore-platform-reqs"})
	if err := run(ctx, dir, o.Out, o.Env, composer...); err != nil {
		return Artifact{}, err
	}
	if o.Hook != "" {
		env := append([]string{"TRELLIS_BUILD_DIR=" + dir, "TRELLIS_COMMIT=" + commit}, o.Env...)
		if err := run(ctx, dir, o.Out, env, "/bin/sh", "-e", o.Hook); err != nil {
			return Artifact{}, fmt.Errorf("build hook: %w", err)
		}
	}
	data, files, err := pack(dir)
	if err != nil {
		return Artifact{}, err
	}
	sum := sha256.Sum256(data)
	return Artifact{Data: data, SHA256: hex.EncodeToString(sum[:]), Commit: commit, Files: files}, nil
}

func short(commit string) string {
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func output(ctx context.Context, dir string, argv ...string) (string, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

func run(ctx context.Context, dir string, out io.Writer, env []string, argv ...string) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Env = append(os.Environ(), env...)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(argv, " "), err)
	}
	return nil
}

// extract unpacks a git archive into dir.
func extract(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := path.Clean(h.Name)
		if name == "." || path.IsAbs(name) || strings.HasPrefix(name, "../") || name == ".." {
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fs.FileMode(h.Mode)&0o777)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, dst); err != nil {
				return err
			}
		}
	}
}

// pack tars and gzips dir, skipping excluded directories.
func pack(dir string) ([]byte, int, error) {
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	tw := tar.NewWriter(gz)
	files := 0
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() && excluded[d.Name()] {
			return filepath.SkipDir
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		link := ""
		if info.Mode()&fs.ModeSymlink != 0 {
			if link, err = os.Readlink(p); err != nil {
				return err
			}
		}
		h, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		h.Name = filepath.ToSlash(rel)
		h.Uid, h.Gid, h.Uname, h.Gname = 0, 0, "", ""
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			f, err := os.Open(p)
			if err != nil {
				return err
			}
			defer func() { _ = f.Close() }()
			if _, err := io.Copy(tw, f); err != nil {
				return err
			}
			files++
		}
		return nil
	})
	if err != nil {
		return nil, 0, err
	}
	if err := tw.Close(); err != nil {
		return nil, 0, err
	}
	if err := gz.Close(); err != nil {
		return nil, 0, err
	}
	return buf.Bytes(), files, nil
}

// Hooks reads the project's server-side deploy hooks.
func Hooks(projectRoot string) (release.Hooks, string, error) {
	read := func(name string) (string, error) {
		b, err := os.ReadFile(filepath.Join(projectRoot, HooksDir, name))
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return string(b), err
	}
	var h release.Hooks
	var err error
	if h.Release, err = read("release.sh"); err != nil {
		return h, "", err
	}
	if h.After, err = read("after.sh"); err != nil {
		return h, "", err
	}
	build := filepath.Join(projectRoot, HooksDir, "build.sh")
	if _, err := os.Stat(build); err != nil {
		build = ""
	}
	return h, build, nil
}

// Spec describes deploying art for site in env.
func Spec(env *config.Environment, site config.Site, art Artifact, hooks release.Hooks, now time.Time) release.Spec {
	shared := []release.Link{{Path: ".env", Target: "shared/.env"}}
	for _, w := range site.Writable {
		shared = append(shared, release.Link{Path: strings.Trim(w, "/"), Target: "shared/" + config.SharedName(w)})
	}
	return release.Spec{
		Site:      site.Name,
		ID:        now.UTC().Format("20060102150405") + "-" + short(art.Commit),
		Artifact:  art.RemotePath(),
		Shared:    shared,
		PHPUnit:   "php" + env.Server.PHP.Version + "-fpm.service",
		Multisite: site.Multisite.Enabled,
		Hooks:     hooks,
		Env:       []string{"TRELLIS_ENV=" + env.Name, "TRELLIS_COMMIT=" + art.Commit},
		Keep:      5,
	}
}
