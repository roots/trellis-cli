package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/state"
)

// Op is what an action does.
type Op string

const (
	OpCreate Op = "create" // resource is missing
	OpUpdate Op = "update" // resource exists but differs
	OpRemove Op = "remove" // desired absent (e.g. a package's default config)
	OpRun    Op = "run"    // exec whose inputs changed
	OpPrune  Op = "prune"  // no longer in the manifest; Trellis owned it
)

// Action is one planned change.
type Action struct {
	Op       Op                `json:"op"`
	Resource manifest.Resource `json:"resource"`
	Reasons  []string          `json:"reasons,omitempty"`
}

func (a Action) ID() manifest.ID { return a.Resource.ID() }

// Plan is the ordered set of changes that converges a host to a manifest.
type Plan struct {
	Node         string   `json:"node"`
	ManifestHash string   `json:"manifest_hash"`
	Generation   int      `json:"generation"`
	Actions      []Action `json:"actions"`
	Unchanged    int      `json:"unchanged"`
	Warnings     []string `json:"warnings,omitempty"`
	// Upgradable are installed packages with updates waiting (reported,
	// not part of the plan).
	Upgradable []string `json:"upgradable,omitempty"`
}

// Empty reports whether applying the plan would change nothing.
func (p *Plan) Empty() bool { return len(p.Actions) == 0 }

// Redacted returns a copy of p without secrets, for sending back to the
// client. Its Hash is unchanged.
func (p *Plan) Redacted() *Plan {
	c := *p
	c.Actions = make([]Action, len(p.Actions))
	for i, a := range p.Actions {
		a.Resource = a.Resource.Redacted()
		c.Actions[i] = a
	}
	return &c
}

// Hash binds an approval to a plan: `apply --expect` refuses to run if the
// plan computed on the host differs from the one the user reviewed.
func (p *Plan) Hash() string {
	h := sha256.New()
	h.Write([]byte(p.ManifestHash))
	for _, a := range p.Actions {
		fmt.Fprintf(h, "\n%s %s %s", a.Op, a.ID(), a.Resource.Digest())
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Compute diffs desired against observed. It is a pure function.
//
// Ordering: create/update/remove/run actions follow manifest order (phase,
// then ID). Prune actions follow, in reverse phase order, so services stop
// before their files and packages go away.
func Compute(desired *manifest.Manifest, obs *state.Observed) *Plan {
	p := &Plan{
		Node:         desired.Node,
		ManifestHash: desired.Hash(),
		Generation:   1,
		Warnings:     slices.Clone(desired.Warnings),
		Upgradable:   obs.Upgradable,
	}
	if obs.RebootRequired {
		p.Warnings = append(p.Warnings, "the server needs a reboot to finish installing updates")
	}
	if n := len(obs.Upgradable); n > 0 {
		names := obs.Upgradable
		if n > 8 {
			names = append(slices.Clip(names[:8]), fmt.Sprintf("and %d more", n-8))
		}
		p.Warnings = append(p.Warnings, fmt.Sprintf("updates are waiting for %d packages (%s): unattended-upgrades installs the ones auto_updates allows nightly, `trellis converge upgrade` now", n, strings.Join(names, ", ")))
	}
	prev := obs.Previous
	if prev != nil {
		p.Generation = prev.Number + 1
	}

	for i := range desired.Resources {
		r := desired.Resources[i]
		op, reasons := diff(&r, obs, prev)
		if op == "" {
			p.Unchanged++
			continue
		}
		p.Actions = append(p.Actions, Action{Op: op, Resource: r, Reasons: reasons})
	}

	var prunes []Action
	for id, r := range prev.Owned() {
		if desired.Get(id) != nil {
			continue
		}
		switch r.Kind {
		case manifest.KindDatabase:
			p.Warnings = append(p.Warnings, fmt.Sprintf("database %s is no longer managed; it is kept (drop it manually if intended)", r.Name))
			continue
		case manifest.KindExec:
			continue
		case manifest.KindDatabaseUser:
			// History is redacted, so dropping a user on a remote server
			// needs that server's current credentials from the manifest.
			if srv := r.DatabaseUser.Server; srv != nil {
				live := dbServer(desired, srv.Host)
				if live == nil {
					p.Warnings = append(p.Warnings, fmt.Sprintf("database user %s is no longer managed but %s has no credentials in the manifest; drop it manually if intended", r.Name, srv.Host))
					continue
				}
				u := *r.DatabaseUser
				u.Server = live
				r.DatabaseUser = &u
			}
		}
		if present(&r, obs) {
			prunes = append(prunes, Action{Op: OpPrune, Resource: r, Reasons: []string{"no longer in manifest"}})
		}
	}
	slices.SortFunc(prunes, func(a, b Action) int {
		if a.Resource.Phase != b.Resource.Phase {
			return int(b.Resource.Phase - a.Resource.Phase)
		}
		return strings.Compare(a.ID().String(), b.ID().String())
	})
	p.Actions = append(p.Actions, prunes...)
	return p
}

// diff returns the op needed for r ("" if none) and why.
func diff(r *manifest.Resource, obs *state.Observed, prev *state.Generation) (Op, []string) {
	switch r.Kind {
	case manifest.KindPackage:
		have, ok := obs.Packages[r.Package.Name]
		if !ok {
			return OpCreate, []string{"not installed"}
		}
		if want := r.Package.Version; want != "" && want != have {
			return OpUpdate, []string{fmt.Sprintf("version %s → %s", have, want)}
		}

	case manifest.KindFile:
		f := r.File
		fi := obs.Files[f.Path]
		if f.Absent {
			if fi.Exists {
				return OpRemove, []string{"exists"}
			}
			return "", nil
		}
		if !fi.Exists {
			return OpCreate, []string{"missing"}
		}
		var reasons []string
		if fi.SHA256 != f.SHA256() {
			reasons = append(reasons, "content")
		}
		if fi.Mode != f.Mode {
			reasons = append(reasons, fmt.Sprintf("mode %04o → %04o", fi.Mode, f.Mode))
		}
		if fi.Owner != f.Owner || fi.Group != f.Group {
			reasons = append(reasons, fmt.Sprintf("owner %s:%s → %s:%s", fi.Owner, fi.Group, f.Owner, f.Group))
		}
		if len(reasons) > 0 {
			return OpUpdate, reasons
		}

	case manifest.KindDownload:
		d := r.Download
		fi := obs.Files[d.Path]
		if !fi.Exists {
			return OpCreate, []string{"missing"}
		}
		var reasons []string
		if d.SHA256 != "" && fi.SHA256 != d.SHA256 {
			reasons = append(reasons, "sha256 mismatch")
		}
		if fi.Mode != d.Mode {
			reasons = append(reasons, fmt.Sprintf("mode %04o → %04o", fi.Mode, d.Mode))
		}
		if old := prev.Applied(r.ID()); old != nil && old.Download.URL != d.URL {
			reasons = append(reasons, "url changed")
		}
		if len(reasons) > 0 {
			return OpUpdate, reasons
		}

	case manifest.KindService:
		u := obs.Units[r.Service.Unit]
		var reasons []string
		if r.Service.Enabled != nil && *r.Service.Enabled != u.Enabled() {
			if *r.Service.Enabled {
				reasons = append(reasons, "enable")
			} else {
				reasons = append(reasons, "disable")
			}
		}
		if r.Service.Active != u.Active() {
			if r.Service.Active {
				reasons = append(reasons, "start")
			} else {
				reasons = append(reasons, "stop")
			}
		}
		if len(reasons) > 0 {
			return OpUpdate, reasons
		}

	case manifest.KindDatabase:
		if obs.DBUnknown || !obs.Databases[r.Name] {
			return OpCreate, []string{"missing"}
		}

	case manifest.KindDatabaseUser:
		if obs.DBUnknown || !obs.DBUsers[r.Name] {
			return OpCreate, []string{"missing"}
		}
		if old := prev.Applied(r.ID()); old == nil || old.Digest() != r.Digest() {
			return OpUpdate, []string{"password or grants changed"}
		}

	case manifest.KindCert:
		c := r.Cert
		if !obs.Files[c.CertPath].Exists || !obs.Files[c.KeyPath].Exists {
			return OpCreate, []string{"missing"}
		}
		if c.Provider == manifest.CertLetsEncrypt && !obs.Files[letsEncryptLive(r.Name)].Exists {
			return OpUpdate, []string{"not yet issued by Let's Encrypt"}
		}
		if old := prev.Applied(r.ID()); old == nil || old.Digest() != r.Digest() {
			return OpUpdate, []string{"domains or provider changed"}
		}

	case manifest.KindExec:
		if old := prev.Applied(r.ID()); old == nil {
			return OpRun, []string{"never ran"}
		} else if old.Exec.Inputs != r.Exec.Inputs || !slices.Equal(old.Exec.Command, r.Exec.Command) {
			return OpRun, []string{"inputs changed"}
		} else if old.Redacted().Exec.Stdin != r.Redacted().Exec.Stdin {
			return OpRun, []string{"secret input changed"}
		} else if !maps.Equal(old.Redacted().Exec.Env, r.Redacted().Exec.Env) {
			return OpRun, []string{"env changed"}
		}
	}
	return "", nil
}

// present reports whether a previously owned resource still exists, so
// pruning it would do something.
func present(r *manifest.Resource, obs *state.Observed) bool {
	switch r.Kind {
	case manifest.KindPackage:
		_, ok := obs.Packages[r.Package.Name]
		return ok
	case manifest.KindFile:
		return !r.File.Absent && obs.Files[r.File.Path].Exists
	case manifest.KindDownload:
		return obs.Files[r.Download.Path].Exists
	case manifest.KindService:
		u := obs.Units[r.Service.Unit]
		return u.Active() || u.Enabled()
	case manifest.KindDatabaseUser:
		return obs.DBUnknown || obs.DBUsers[r.Name]
	case manifest.KindCert:
		return obs.Files[r.Cert.CertPath].Exists || obs.Files[r.Cert.KeyPath].Exists
	}
	return false
}

// dbServer returns the credentials m uses for the database server at host.
func dbServer(m *manifest.Manifest, host string) *manifest.DBServer {
	for i := range m.Resources {
		var s *manifest.DBServer
		switch r := &m.Resources[i]; {
		case r.Database != nil:
			s = r.Database.Server
		case r.DatabaseUser != nil:
			s = r.DatabaseUser.Server
		}
		if s != nil && s.Host == host {
			return s
		}
	}
	return nil
}
