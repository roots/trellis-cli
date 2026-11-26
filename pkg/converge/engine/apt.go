package engine

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/roots/trellis-cli/pkg/converge/host"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
)

// Package installs take most of a first provision, so they get three
// speedups:
//
//   - Prefetch: apt downloads from each host over one connection, one file
//     at a time. The agent downloads the same .debs in parallel into apt's
//     cache first; apt then verifies every cached file against the signed
//     index and fetches anything missing or wrong itself, so a prefetch
//     can only save time, never change what's installed.
//
//   - Deferred starts: unless the install upgrades or removes an installed
//     package that ships a daemon, a policy-rc.d makes packages enable
//     their daemons without starting them. Trellis's config is written
//     next, and each daemon starts once with it, rather than starting with
//     the package defaults and restarting at the end. Starts the packages
//     asked for are logged and done after the files and certs phases (so
//     the data phase has its database server), as the packages would have
//     done them, unless Trellis wants the unit stopped. (Upgrading a daemon
//     is excluded because policy-rc.d would also block the restart the
//     upgrade needs. Library upgrades, which fresh images always have, are
//     fine.)
//
//   - --force-unsafe-io on a server's first converge: dpkg skips an fsync
//     per unpacked file, and one sync follows the install. Only a crash
//     during that install could leave a truncated file, on a server that
//     is being built anyway; later installs on a running server use
//     dpkg's safe default.

const (
	policyRCD = "/usr/sbin/policy-rc.d"
	// deniedLog lists the actions policy-rc.d blocked. It's in /run so a
	// reboot, which starts enabled units anyway, clears it.
	deniedLog   = "/run/trellis/policy-rc.d.log"
	policyMark  = "# Trellis: defers daemon starts during a package install."
	prefetchers = 8
)

var policyScript = "#!/bin/sh\n" + policyMark + "\n" +
	"mkdir -p " + path.Dir(deniedLog) + " && echo \"$*\" >> " + deniedLog + "\nexit 101\n"

var aptBaseArgs = []string{"-y", "-q", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}

func aptPackageArgs(pkgs []manifest.Package) []string {
	args := []string{"--"}
	for _, p := range pkgs {
		if p.Version != "" {
			args = append(args, p.Name+"="+p.Version)
		} else {
			args = append(args, p.Name)
		}
	}
	return args
}

func aptInstallCmd(pkgs []manifest.Package, extra ...string) host.Cmd {
	argv := slices.Concat([]string{"apt-get", "install"}, aptBaseArgs, extra, aptPackageArgs(pkgs))
	return host.Cmd{Argv: argv, Env: aptEnv}
}

// instRe matches a simulated install line: "Inst <name> (<new> …)" for a
// new package, "Inst <name> [<old>] (<new> …)" for an upgrade.
var instRe = regexp.MustCompile(`^Inst (\S+) (\[|\()`)

// parseSimulate returns the installed packages an `apt-get install -s`
// would upgrade or remove. ok is false unless every Inst line, and at
// least one, is in a known format: callers then assume the worst, so a
// change in apt's output can't make an upgrade look safe.
func parseSimulate(out []byte) (touched []string, ok bool) {
	insts := 0
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		f := strings.Fields(line)
		switch {
		case len(f) == 0:
		case f[0] == "Inst":
			insts++
			m := instRe.FindStringSubmatch(line)
			if m == nil {
				return nil, false
			}
			if m[2] == "[" {
				touched = append(touched, m[1])
			}
		case f[0] == "Remv":
			if len(f) < 2 {
				return nil, false
			}
			touched = append(touched, f[1])
		}
	}
	return touched, insts > 0
}

// touchedInstalled returns the installed packages installing pkgs would
// upgrade or remove; ok as for parseSimulate.
func (r *run) touchedInstalled(pkgs []manifest.Package) (touched []string, ok bool, err error) {
	out, err := r.h.Run(r.ctx, host.Cmd{Argv: slices.Concat([]string{"apt-get", "install", "-s", "-q"}, aptPackageArgs(pkgs)), Env: aptEnv})
	if err != nil {
		return nil, false, err
	}
	touched, ok = parseSimulate(out)
	return touched, ok, nil
}

// upgradableCmd simulates upgrading everything (dist-upgrade, so updates
// that need new dependencies count too).
var upgradableCmd = host.Cmd{Argv: []string{"apt-get", "dist-upgrade", "-s", "-q"}, Env: aptEnv}

// parseUpgradable returns the installed packages a simulated upgrade
// would upgrade.
func parseUpgradable(out []byte) []string {
	var names []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if m := instRe.FindStringSubmatch(sc.Text()); m != nil && m[2] == "[" {
			names = append(names, m[1])
		}
	}
	return names
}

// daemonFileRe matches the files that make a package's upgrade stop or
// restart something: systemd units and init scripts.
var daemonFileRe = regexp.MustCompile(`^(/usr)?/lib/systemd/system/[^/]+\.(service|socket|timer|path)$|^/etc/init\.d/`)

// shipDaemons reports whether any of the installed packages names has a
// systemd unit or init script.
func (r *run) shipDaemons(names []string) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	out, err := r.h.Run(r.ctx, host.Cmd{Argv: slices.Concat([]string{"dpkg-query", "-L", "--"}, names)})
	if err != nil {
		return true, nil // unknown: be safe
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if daemonFileRe.MatchString(sc.Text()) {
			return true, nil
		}
	}
	return false, nil
}

type debURI struct {
	url, name string
	size      int64
}

// parsePrintURIs parses `apt-get --print-uris` lines:
// 'URL' FILENAME SIZE HASH.
func parsePrintURIs(out []byte) []debURI {
	var debs []debURI
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || !strings.HasPrefix(f[0], "'") {
			continue
		}
		u := strings.Trim(f[0], "'")
		name := f[1]
		size, _ := strconv.ParseInt(f[2], 10, 64)
		if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
			continue // file:, cdrom: and the like are local already
		}
		if name != path.Base(name) || strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".deb") {
			continue
		}
		debs = append(debs, debURI{url: u, name: name, size: size})
	}
	return debs
}

// prefetch downloads the .debs installing pkgs needs into apt's cache, in
// parallel. Failures are ignored: apt downloads whatever is missing.
func (r *run) prefetch(pkgs []manifest.Package) {
	if cfg, err := r.h.Run(r.ctx, host.Cmd{Argv: []string{"apt-config", "dump", "Acquire"}}); err != nil ||
		bytes.Contains(bytes.ToLower(cfg), []byte("proxy")) {
		return // apt knows how to reach its mirrors; the agent might not
	}
	out, err := r.h.Run(r.ctx, host.Cmd{Argv: slices.Concat([]string{"apt-get", "install", "-qq", "--print-uris"}, aptPackageArgs(pkgs)), Env: aptEnv})
	if err != nil {
		return
	}
	debs := parsePrintURIs(out)
	if len(debs) == 0 {
		return
	}
	var total int64
	for _, d := range debs {
		total += d.size
	}
	r.log("fetch", fmt.Sprintf("%d packages (%.1f MB)", len(debs), float64(total)/1e6), "")
	work := make(chan debURI)
	var wg sync.WaitGroup
	for range min(prefetchers, len(debs)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range work {
				data, err := r.h.Fetch(r.ctx, d.url)
				if err != nil || (d.size > 0 && int64(len(data)) != d.size) {
					continue
				}
				_ = r.h.WriteFile("/var/cache/apt/archives/"+d.name, data, 0o644, "root", "root", nil)
			}
		}()
	}
	for _, d := range debs {
		work <- d
	}
	close(work)
	wg.Wait()
}

// removeStalePolicy removes a policy-rc.d Trellis left behind (the agent
// died mid-install), which would otherwise keep blocking every later
// package's restarts. Anyone else's policy-rc.d is left alone.
func (r *run) removeStalePolicy() error {
	b, err := r.h.ReadFile(policyRCD)
	if err != nil || !bytes.Contains(b, []byte(policyMark)) {
		return nil
	}
	return r.h.Remove(policyRCD)
}

// canDeferStarts reports whether installing pkgs can defer daemon starts:
// apt's simulation is understood, it upgrades or removes no installed
// package that ships a daemon, and no one else's policy-rc.d is in place.
func (r *run) canDeferStarts(pkgs []manifest.Package) (bool, error) {
	if info, err := r.h.Stat(policyRCD, false); err != nil || info.Exists {
		return false, err
	}
	touched, ok, err := r.touchedInstalled(pkgs)
	if err != nil || !ok {
		return false, err
	}
	daemons, err := r.shipDaemons(touched)
	return !daemons, err
}

// installDeferred installs pkgs with daemon starts deferred.
func (r *run) installDeferred(pkgs []manifest.Package) error {
	if err := r.h.WriteFile(policyRCD, []byte(policyScript), 0o755, "root", "root", nil); err != nil {
		return err
	}
	return errors.Join(r.install(pkgs), r.h.Remove(policyRCD))
}

// install runs apt-get install, with unsafe I/O (then a sync) on a
// server's first converge.
func (r *run) install(pkgs []manifest.Package) error {
	if !r.firstConverge {
		_, err := r.h.Run(r.ctx, aptInstallCmd(pkgs))
		return err
	}
	_, err := r.h.Run(r.ctx, aptInstallCmd(pkgs, "-o", "Dpkg::Options::=--force-unsafe-io"))
	_, syncErr := r.h.Run(r.ctx, host.Cmd{Argv: []string{"sync"}})
	return errors.Join(err, syncErr)
}

// unitSuffixes are the unit types policy-rc.d callers name explicitly;
// anything else is an init script name for a .service.
var unitSuffixes = []string{".service", ".socket", ".timer", ".path", ".target", ".mount"}

// parseDenied returns the units policy-rc.d was asked to start. Lines are
// policy-rc.d's arguments: "<id> <action> [runlevel]", where id is a unit
// (deb-systemd-invoke) or an init script name (invoke-rc.d), and action
// may be parenthesized (optional).
func parseDenied(log []byte) []string {
	var units []string
	sc := bufio.NewScanner(bytes.NewReader(log))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		action := strings.Trim(f[1], "()")
		if action != "start" && action != "restart" {
			continue
		}
		unit := f[0]
		if !slices.ContainsFunc(unitSuffixes, func(s string) bool { return strings.HasSuffix(unit, s) }) {
			unit += ".service"
		}
		if !slices.Contains(units, unit) {
			units = append(units, unit)
		}
	}
	return units
}

// startDeferred starts what packages asked to start during a deferred
// install, now that Trellis's files and certificates are in place: units
// that are enabled and not running, as the package's own start would have
// done, except those Trellis wants stopped. They count as freshly started,
// so their reload/restart notifications are skipped.
func (r *run) startDeferred() error {
	log, err := r.h.ReadFile(deniedLog)
	if err != nil || len(log) == 0 {
		return nil
	}
	var units []string
	for _, u := range parseDenied(log) {
		if svc := r.desired.Get(manifest.ID{Kind: manifest.KindService, Name: u}); svc == nil || svc.Service.Active {
			units = append(units, u)
		}
	}
	if len(units) > 0 {
		out, err := r.h.Run(r.ctx, systemctlShowCmd(units))
		if err != nil {
			return err
		}
		live := parseSystemctlShow(out)
		for _, u := range units {
			if s, ok := live[u]; !ok || s.Active() || (!s.Enabled() && !r.wantsActive(u)) {
				continue
			}
			r.log("start", u, "deferred from package install")
			if _, err := r.h.Run(r.ctx, host.Cmd{Argv: []string{"systemctl", "start", u}}); err != nil {
				return err
			}
			r.started[u] = true
		}
	}
	return r.h.Remove(deniedLog)
}

// wantsActive reports whether the manifest wants unit running.
func (r *run) wantsActive(unit string) bool {
	svc := r.desired.Get(manifest.ID{Kind: manifest.KindService, Name: unit})
	return svc != nil && svc.Service.Active
}
