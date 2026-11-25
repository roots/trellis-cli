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
	"maps"
	"slices"
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
	// Project is the project root (containing server/, keys/, exec inputs).
	Project fs.FS
	// Version is the trellis-cli version, used for ejected-template checks.
	Version string
}

// Compile returns one finalized manifest per node, in node order. All
// errors (missing secrets, ownership conflicts, template errors) are
// collected and returned together.
func Compile(in Input) ([]*manifest.Manifest, error) {
	if in.Lock == nil {
		in.Lock = lock.New()
	}
	if in.Secrets == nil {
		in.Secrets = secrets.Map{}
	}
	codename, err := ubuntuCodename(in.Lock.OS)
	if err != nil {
		return nil, err
	}

	var overrides fs.FS
	if in.Project != nil {
		if sub, err := fs.Sub(in.Project, "server/ejected"); err == nil {
			overrides = sub
		}
	}
	tpl := templates.NewSet(overrides)
	ejectWarnings := checkEjected(tpl, in.Lock)

	var errs []error
	var out []*manifest.Manifest
	for _, node := range in.Env.Nodes {
		b := &builder{
			in:       in,
			env:      in.Env,
			node:     node,
			tpl:      tpl,
			codename: codename,
			m:        manifest.New(node.Name, slices.Clone(node.Roles)),
			pkgs:     map[string]bool{},
		}
		b.m.Warnings = append(b.m.Warnings, ejectWarnings...)
		b.compile()
		if len(b.errs) > 0 {
			errs = append(errs, fmt.Errorf("node %s:\n  %w", node.Name, indent(errors.Join(b.errs...))))
			continue
		}
		b.m.Finalize()
		out = append(out, b.m)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

func indent(err error) error {
	return errors.New(strings.ReplaceAll(err.Error(), "\n", "\n  "))
}

// builder accumulates one node's manifest and its errors.
type builder struct {
	in       Input
	env      *config.Environment
	node     config.Node
	tpl      *templates.Set
	codename string
	m        *manifest.Manifest
	errs     []error
	pkgs     map[string]bool
}

func (b *builder) compile() {
	b.base()
	if b.node.HasRole(config.RoleWeb) {
		b.webStack()
		b.services()
		b.userExec()
	}
	if b.node.HasRole(config.RoleDB) {
		b.db()
	}
	b.dropIns()
}

func (b *builder) isWeb() bool { return b.node.HasRole(config.RoleWeb) }
func (b *builder) isDB() bool  { return b.node.HasRole(config.RoleDB) }

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

func ubuntuCodename(os string) (string, error) {
	switch os {
	case "", "ubuntu-24.04":
		return "noble", nil
	case "ubuntu-22.04":
		return "jammy", nil
	}
	return "", fmt.Errorf("trellis.lock: unsupported os %q (supported: ubuntu-22.04, ubuntu-24.04)", os)
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

// sortedKeys is a small helper for deterministic iteration.
func sortedKeys[V any](m map[string]V) []string {
	return slices.Sorted(maps.Keys(m))
}
