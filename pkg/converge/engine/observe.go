// Package engine observes a host, plans the changes needed to reach a
// manifest, and applies them.
//
//	observed := Observe(host, desired)        // imperative, read-only
//	plan     := Plan(desired, observed)       // pure
//	result   := Apply(host, desired, plan)    // imperative
//
// Plan is a pure function so every planning decision is unit-testable;
// Observe and Apply only talk to the machine through host.Host.
package engine

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"

	"github.com/roots/trellis-cli/pkg/converge/host"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/state"
)

// LoadGeneration reads the last applied generation (nil if none).
func LoadGeneration(h host.Host) (*state.Generation, error) {
	b, err := h.ReadFile(state.CurrentPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return state.UnmarshalGeneration(b)
}

// Observe collects the state of every resource in desired and in the
// previous generation (so pruning decisions are based on reality too).
// It issues a constant number of commands regardless of manifest size:
// one dpkg-query, one systemctl show, and two queries per database server.
func Observe(ctx context.Context, h host.Host, desired *manifest.Manifest) (*state.Observed, error) {
	prev, err := LoadGeneration(h)
	if err != nil {
		return nil, fmt.Errorf("read previous generation: %w", err)
	}
	obs := state.NewObserved()
	obs.Previous = prev
	if fi, err := h.Stat(state.RebootRequiredPath, false); err == nil && fi.Exists {
		obs.RebootRequired = true
	}

	resources := slices.Clone(desired.Resources)
	for _, r := range prev.Owned() {
		if desired.Get(r.ID()) == nil {
			resources = append(resources, r)
		}
	}

	paths := map[string]bool{} // path → hash wanted
	pkgs := map[string]bool{}
	units := map[string]bool{}
	dbServers := map[string]*manifest.DBServer{}
	for i := range resources {
		r := &resources[i]
		switch r.Kind {
		case manifest.KindFile:
			paths[r.File.Path] = paths[r.File.Path] || !r.File.Absent
		case manifest.KindDownload:
			paths[r.Download.Path] = paths[r.Download.Path] || r.Download.SHA256 != ""
		case manifest.KindCert:
			paths[r.Cert.CertPath] = paths[r.Cert.CertPath] || false
			paths[r.Cert.KeyPath] = paths[r.Cert.KeyPath] || false
			if r.Cert.Provider == manifest.CertLetsEncrypt {
				paths[letsEncryptLive(r.Name)] = false
			}
		case manifest.KindPackage:
			pkgs[r.Package.Name] = true
		case manifest.KindService:
			units[r.Service.Unit] = true
		case manifest.KindDatabase:
			dbServers[serverKey(r.Database.Server)] = r.Database.Server
		case manifest.KindDatabaseUser:
			dbServers[serverKey(r.DatabaseUser.Server)] = r.DatabaseUser.Server
		}
	}

	for _, p := range slices.Sorted(maps.Keys(paths)) {
		fi, err := h.Stat(p, paths[p])
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		obs.Files[p] = fi
	}

	if len(pkgs) > 0 {
		// dpkg-query exits non-zero when some names are unknown but still
		// reports the rest, so the output is parsed regardless.
		out, _ := h.Run(ctx, dpkgQueryCmd(slices.Sorted(maps.Keys(pkgs))))
		obs.Packages = parseDpkg(out)
		// Only reported, so a failure just means nothing to report.
		if out, err := h.Run(ctx, upgradableCmd); err == nil {
			obs.Upgradable = parseUpgradable(out)
		}
	}

	if len(units) > 0 {
		out, err := h.Run(ctx, systemctlShowCmd(slices.Sorted(maps.Keys(units))))
		if err != nil {
			return nil, err
		}
		obs.Units = parseSystemctlShow(out)
	}

	for _, key := range slices.Sorted(maps.Keys(dbServers)) {
		server := dbServers[key]
		schemas, err1 := h.Run(ctx, mariadbCmd(server, selectSchemasSQL))
		users, err2 := h.Run(ctx, mariadbCmd(server, selectUsersSQL))
		if err1 != nil || err2 != nil {
			obs.DBUnknown = true
			continue
		}
		for _, s := range parseLines(schemas) {
			obs.Databases[dbResourceName(server, s)] = true
		}
		for _, u := range parseLines(users) {
			obs.DBUsers[dbResourceName(server, u)] = true
		}
	}
	return obs, nil
}
