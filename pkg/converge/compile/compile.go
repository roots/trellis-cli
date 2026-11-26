// Package compile turns a resolved environment into one manifest per node.
//
// Compile is a pure function of its Input: no network, no clock, no host
// access. The same trellis.yml + trellis.lock + secrets + project files
// always produce byte-identical manifests (and therefore the same hash),
// which is what makes plans reviewable and no-op runs instant.
package compile

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/lock"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
	"github.com/roots/trellis-cli/pkg/converge/templates"
)

// Input is everything compilation depends on.
type Input struct {
	Env     *config.Environment
	Lock    *lock.Lock
	Secrets secrets.Store
	// Project is the project root (containing server/ and keys/).
	Project fs.FS
	// Environments are every environment in trellis.yml, to catch typos in
	// server/env/<name>/ (optional).
	Environments []string
	// Version is the trellis-cli version, used for ejected-template checks.
	Version string
}

// Compile returns the environment's finalized manifest (one server per
// environment). All errors (missing secrets, ownership conflicts, template
// errors) are collected and returned together.
func Compile(in Input) (*manifest.Manifest, error) {
	if in.Lock == nil {
		in.Lock = lock.New()
	}
	if in.Secrets == nil {
		in.Secrets = secrets.Map{}
	}
	plat, err := platformFor(in.Env.Server.OS)
	if err != nil {
		return nil, err
	}
	if in.Lock.AptSnapshot != "" && plat.distro != "ubuntu" {
		return nil, fmt.Errorf("trellis.lock: apt_snapshot uses snapshot.ubuntu.com, which doesn't serve %s", in.Env.Server.OS)
	}

	var overrides fs.FS
	if in.Project != nil {
		if sub, err := fs.Sub(in.Project, "server/ejected"); err == nil {
			overrides = sub
		}
	}
	tpl := templates.NewSet(overrides)
	ejectWarnings := checkEjected(tpl, in.Lock)

	roles := []string{"web", "db"}
	if !in.Env.LocalDB() {
		roles = []string{"web"}
	}
	b := &builder{
		in:       in,
		env:      in.Env,
		tpl:      tpl,
		codename: plat.codename,
		distro:   plat.distro,
		m:        manifest.New(in.Env.ServerName(), roles),
		pkgs:     map[string]bool{},
	}
	b.m.OS = in.Env.Server.OS
	b.m.Warnings = append(b.m.Warnings, ejectWarnings...)
	b.compile()
	if len(b.errs) > 0 {
		return nil, fmt.Errorf("  %w", indent(errors.Join(b.errs...)))
	}
	b.m.Finalize()
	return b.m, nil
}

func indent(err error) error {
	return errors.New(strings.ReplaceAll(err.Error(), "\n", "\n  "))
}

// builder accumulates the manifest and its errors.
type builder struct {
	in       Input
	env      *config.Environment
	tpl      *templates.Set
	codename string // e.g. noble, trixie
	distro   string // ubuntu or debian: the path in nginx.org and MariaDB repos
	m        *manifest.Manifest
	errs     []error
	pkgs     map[string]bool
}

func (b *builder) compile() {
	b.base()
	b.webStack()
	b.externalDatabases()
	if b.env.LocalDB() {
		b.db()
	}
	// User files and commands come last: they may not take over a path
	// Trellis manages, and commands watch files from either.
	b.serverFiles()
	b.commands()
}

func (b *builder) errorf(format string, args ...any) {
	b.errs = append(b.errs, fmt.Errorf(format, args...))
}

func (b *builder) warnf(format string, args ...any) {
	b.m.Warnings = append(b.m.Warnings, fmt.Sprintf(format, args...))
}

func (b *builder) add(rs ...manifest.Resource) {
	if err := b.m.Add(rs...); err != nil {
		b.errs = append(b.errs, err)
	}
}

// render executes a template; errors are collected and "" returned.
func (b *builder) render(name string, data any) string {
	s, err := b.tpl.Render(name, data)
	if err != nil {
		b.errs = append(b.errs, err)
	}
	return s
}

// secret resolves a value that may be a secret reference.
func (b *builder) secret(v, where string) string {
	s, err := secrets.Resolve(b.in.Secrets, v)
	if err != nil {
		b.errorf("%s: %v", where, err)
	}
	return s
}

// required looks up a generated secret by path.
func (b *builder) required(path ...string) string {
	v, ok := b.in.Secrets.Lookup(path...)
	if !ok {
		b.errorf("secret %s is missing (run `trellis converge secrets init %s`)", strings.Join(path, "."), b.env.Name)
	}
	return v
}

// pkg adds apt packages, deduplicated (user packages may overlap core
// ones), pinned from the lockfile when available.
func (b *builder) pkg(names ...string) {
	for _, n := range names {
		if b.pkgs[n] {
			continue
		}
		b.pkgs[n] = true
		b.add(manifest.NewPackage(n, b.in.Lock.Packages[n]))
	}
}

type platform struct{ distro, codename string }

// platforms are the supported operating systems. Third-party repositories
// (nginx.org, packages.sury.org, MariaDB) serve all of them.
var platforms = map[string]platform{
	"ubuntu-24.04": {"ubuntu", "noble"},
	"ubuntu-26.04": {"ubuntu", "resolute"},
	"debian-12":    {"debian", "bookworm"},
	"debian-13":    {"debian", "trixie"},
}

func platformFor(os string) (platform, error) {
	if p, ok := platforms[os]; ok {
		return p, nil
	}
	return platform{}, fmt.Errorf("unsupported os %q", os)
}

// checkEjected warns when an ejected template's built-in base changed
// since it was ejected.
func checkEjected(tpl *templates.Set, l *lock.Lock) []string {
	var warnings []string
	for _, name := range templates.Names() {
		if !tpl.Ejected(name) {
			continue
		}
		cur, _ := templates.SHA256(name)
		e, ok := l.Ejected[name]
		switch {
		case !ok:
			warnings = append(warnings, fmt.Sprintf("template %s is ejected but not recorded in trellis.lock; upstream changes can't be detected", name))
		case e.SHA256 != cur:
			warnings = append(warnings, fmt.Sprintf("template %s changed upstream since you ejected it (from %s); compare with `trellis converge eject --show %s`", name, e.Base, name))
		}
	}
	return warnings
}
