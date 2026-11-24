package doctor

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

const (
	// Package lists older than this can't confirm a package is current.
	aptListsStaleAfter = 48 * time.Hour
	// unattended-upgrades runs daily; flag it when it hasn't for longer.
	uuStaleAfter = 48 * time.Hour
	// A PHP release not reaching the server's apt source after this long
	// suggests the source itself isn't being updated.
	phpSourceLagAfter = 7 * 24 * time.Hour
	// Warn ahead of an Ubuntu release's end of standard support.
	osEOLWarnBefore = 180 * 24 * time.Hour
)

// projectVars are values resolved from the Trellis project for a host, the
// same way `trellis provision` would resolve them. Empty means unknown.
type projectVars struct {
	PHPVersion           string `json:"php_version"`
	MariaDBVersion       string `json:"mariadb_version"`
	NginxPackage         string `json:"nginx_package"`
	MariaDBServerPackage string `json:"mariadb_server_package"`
}

// evaluate checks a host's facts. Offline skips upstream release lookups.
func evaluate(f *hostFacts, vars projectVars, offline bool) HostResult {
	e := &evaluator{f: f, vars: vars, offline: offline}
	e.host.staleLists = f.AptUpdated.IsZero() || e.since(f.AptUpdated) > aptListsStaleAfter

	e.osRelease()
	e.nginx()
	e.phpFPM()
	e.mariaDB()
	e.reboot()
	e.unattendedUpgrades()

	return e.host
}

type evaluator struct {
	f       *hostFacts
	vars    projectVars
	offline bool
	host    HostResult
}

func (e *evaluator) add(r Result) {
	e.host.Results = append(e.host.Results, r)
}

func (e *evaluator) since(t time.Time) time.Duration {
	return e.f.Now.Sub(t)
}

// addPackageResult records a service's package result. A withheld service is
// never upgraded, and the apt source is listed last for any package that
// isn't current.
func (e *evaluator) addPackageResult(s *Service, r Result, p *aptPackage, upgradable bool, withheldBecause string) {
	if withheldBecause != "" {
		e.host.withheld = append(e.host.withheld, s)
		if upgradable {
			r.Details = append(r.Details, "No upgrade command suggested "+withheldBecause)
			upgradable = false
		}
	}

	if p != nil && p.Origin != "" && r.Status != StatusOK && r.Status != StatusSkip {
		r.Details = append(r.Details, "apt source: "+p.Origin)
	}

	e.add(r)

	if upgradable {
		e.host.upgrades = append(e.host.upgrades, s)
	}
}

func (e *evaluator) osRelease() {
	r := Result{ID: "ubuntu-release", Title: "Ubuntu"}
	name := e.f.OSName
	if name == "" {
		name = strings.TrimSpace(e.f.OSID + " " + e.f.OSVersion)
	}
	eol := e.f.OSEOL.Format(time.DateOnly)

	switch {
	case name == "":
		r.Status, r.Message = StatusSkip, "OS unknown (no /etc/os-release)"
	case e.f.OSID != "ubuntu":
		r.Status, r.Message = StatusSkip, name+" is not Ubuntu"
	case e.f.OSEOL.IsZero():
		r.Status, r.Message = StatusSkip, name+" (support dates unknown)"
	case !e.f.Now.Before(e.f.OSEOL):
		r.Status, r.Message = StatusFail, fmt.Sprintf("%s reached end of standard support on %s", name, eol)
		r.Details = []string{"Ubuntu no longer publishes regular security updates for this release (Ubuntu Pro ESM aside)"}
	case e.f.OSEOL.Sub(e.f.Now) < osEOLWarnBefore:
		r.Status, r.Message = StatusWarn, fmt.Sprintf("%s standard support ends %s", name, eol)
	default:
		r.Status, r.Message = StatusOK, fmt.Sprintf("%s · supported until %s", name, eol)
	}

	e.add(r)
}

func heldDetail(name string) string {
	return fmt.Sprintf("%s is held (apt-mark hold), so provisioning won't upgrade it; run `sudo apt-mark unhold %s` on the server first", name, name)
}

// pkgResult evaluates installed vs candidate for a service's package.
// upgradable is true when provisioning with the package state set to latest
// would (or, with stale package lists, might) update it.
func (e *evaluator) pkgResult(s *Service, name string) (r Result, p *aptPackage, upgradable bool) {
	r = Result{ID: s.Tag, Title: name}
	p = e.f.Packages[name]

	if p == nil || p.Installed == "" {
		r.Status, r.Message = StatusSkip, "not installed"
		return r, nil, false
	}

	installed := upstreamVersion(p.Installed)

	switch {
	case p.AptError:
		r.Status = StatusUnknown
		r.Message = installed + " installed; apt-cache failed on the server, so available updates are unknown"
		r.Details = []string{"Run `apt-cache policy` on the server to see the error; a broken apt source stops all updates"}
	case p.Candidate == "":
		r.Status, r.Message = StatusWarn, installed+" installed, but no version is available from the server's apt sources"
	case p.Cmp == "lt":
		from, to := displayVersions(p.Installed, p.Candidate)
		r.Status, r.Message = StatusFail, fmt.Sprintf("%s → %s available", from, to)
		upgradable = true
	case p.Cmp == "gt":
		from, to := displayVersions(p.Installed, p.Candidate)
		r.Status, r.Message = StatusWarn, fmt.Sprintf("%s installed is newer than %s available; the apt source may have changed", from, to)
	case p.Cmp == "eq" && p.Origin == "":
		r.Status, r.Message = StatusWarn, installed+" installed, but it isn't available from any of the server's apt sources, so it won't get updates"
	case p.Cmp == "eq" && e.host.staleLists:
		age := "age unknown"
		if !e.f.AptUpdated.IsZero() {
			age = "last updated " + humanDuration(e.since(e.f.AptUpdated)) + " ago"
		}
		r.Status, r.Message = StatusUnknown, fmt.Sprintf("%s (apt package lists %s; can't confirm it's the latest)", installed, age)
		// The Trellis roles refresh the apt cache before installing, so the
		// upgrade command picks up anything newer (or does nothing).
		upgradable = true
	case p.Cmp == "eq":
		r.Status, r.Message = StatusOK, installed
	default:
		r.Status, r.Message = StatusUnknown, fmt.Sprintf("%s installed (couldn't compare with %s)", installed, upstreamVersion(p.Candidate))
	}

	if upgradable && p.Held {
		r.Details = append(r.Details, heldDetail(name))
		upgradable = false
	}

	return r, p, upgradable
}

func (e *evaluator) nginx() {
	r, p, upgradable := e.pkgResult(nginxService, e.vars.NginxPackage)
	e.addPackageResult(nginxService, r, p, upgradable, "")
}

// unknownVersion is why an upgrade is withheld when the project's version of
// a service isn't known: provisioning might switch it.
func unknownVersion(s *Service) string {
	return fmt.Sprintf("because the project's %s couldn't be determined", s.VersionVar)
}

// versionDrift reports a server running a different version series than the
// project's, which provisioning would switch, and returns why an upgrade is
// withheld.
func (e *evaluator) versionDrift(s *Service, server, target, consequence string) string {
	e.add(Result{
		ID:      s.Tag + "-version",
		Title:   s.Name + " version",
		Status:  StatusWarn,
		Message: fmt.Sprintf("Server runs %s %s but the project's %s is %s", s.Name, server, s.VersionVar, target),
		Details: []string{consequence, fmt.Sprintf("To stay on %s %s, set %s: \"%s\" in group_vars", s.Name, server, s.VersionVar, server)},
	})

	return fmt.Sprintf("until the %s version mismatch is resolved", s.Name)
}

// phpFPM evaluates the PHP-FPM branch serving sites, version drift against
// the project's php_version, and the latest upstream release for the branch.
func (e *evaluator) phpFPM() {
	s := phpService
	branches := e.f.phpBranches()
	if len(branches) == 0 {
		e.add(Result{ID: s.Tag, Title: "PHP-FPM", Status: StatusSkip, Message: "not installed"})
		return
	}

	target := e.vars.PHPVersion
	serving := e.servingPHPBranch(branches, target)

	withheld := ""
	switch {
	case target == "":
		withheld = unknownVersion(s)
	case serving != target:
		withheld = e.versionDrift(s, serving, target, "Provisioning will switch sites to PHP "+target)
	}

	r, p, upgradable := e.pkgResult(s, "php"+serving+"-fpm")

	var rel *Release
	if !e.offline && s.LatestRelease != nil && p != nil {
		var err error
		if rel, err = s.LatestRelease(serving); err != nil {
			// Not compared with upstream, so not confirmed current.
			if r.Status == StatusOK {
				r.Status = StatusUnknown
			}
			r.Details = append(r.Details, fmt.Sprintf("Couldn't check php.net's latest PHP %s release: %v", serving, err))
		}
	}

	e.checkPHPRelease(&r, p, rel)
	e.addPackageResult(s, r, p, upgradable, withheld)

	if rel != nil && rel.Supported != nil && !*rel.Supported {
		e.add(Result{
			ID:      s.Tag + "-eol",
			Title:   "PHP " + serving,
			Status:  StatusWarn,
			Message: fmt.Sprintf("PHP %s is no longer supported upstream and gets no security fixes", serving),
		})
	}
}

// servingPHPBranch picks the PHP-FPM branch that's serving sites: the only
// active one, else the project's php_version if active, else the highest
// active, else the project's php_version if installed, else the highest
// installed.
func (e *evaluator) servingPHPBranch(branches []string, target string) string {
	var active []string
	for _, b := range branches {
		if e.f.PHPFPM[b] == "active" {
			active = append(active, b)
		}
	}

	switch {
	case len(active) == 1:
		return active[0]
	case target != "" && slices.Contains(active, target):
		return target
	case len(active) > 1:
		return active[len(active)-1]
	case target != "" && slices.Contains(branches, target):
		return target
	default:
		return branches[len(branches)-1]
	}
}

// checkPHPRelease compares the installed PHP with php.net's latest release
// for the branch, which catches what apt can't: a release that isn't (yet) in
// the server's apt source, or a source that's no longer updated.
func (e *evaluator) checkPHPRelease(r *Result, p *aptPackage, rel *Release) {
	if p == nil || rel == nil {
		return
	}

	installed := upstreamVersion(p.Installed)
	if c, ok := compareVersions(installed, rel.Version); !ok || c >= 0 {
		return
	}

	// php.net only publishes the release date, not the time.
	released := fmt.Sprintf("PHP %s was released on %s", rel.Version, rel.Date.Format(time.DateOnly))
	if rel.Security {
		released = fmt.Sprintf("PHP %s (security release) was released on %s", rel.Version, rel.Date.Format(time.DateOnly))
	}

	if c, ok := compareVersions(upstreamVersion(p.Candidate), rel.Version); ok && c >= 0 {
		// The apt source has it; pkgResult reported the upgrade.
		r.Details = append(r.Details, released)
		return
	}

	// The installed PHP is behind upstream. Whether the apt source lacks the
	// release is only known with working, fresh package lists.
	status, detail := StatusFail, released
	switch {
	case p.AptError || e.host.staleLists:
	case e.since(rel.Date) > phpSourceLagAfter:
		detail += ", but the server's apt source doesn't have it; that source may no longer be updated"
	default:
		status, detail = StatusWarn, released+"; it isn't available from the server's apt source yet"
	}

	// Never downgrade a failure, or turn "couldn't verify" into a warning.
	if status == StatusFail || r.Status == StatusOK {
		r.Status = status
	}
	if p.Cmp != "lt" {
		r.Message = fmt.Sprintf("%s → %s released", installed, rel.Version)
	}
	r.Details = append(r.Details, detail)
}

func (e *evaluator) mariaDB() {
	s := mariaDBService
	r, p, upgradable := e.pkgResult(s, e.vars.MariaDBServerPackage)

	withheld := ""
	if p != nil {
		server, candidate, target := branchOf(p.Installed), branchOf(p.Candidate), e.vars.MariaDBVersion

		switch {
		case target != "" && server != "" && server != target:
			withheld = e.versionDrift(s, server, target,
				fmt.Sprintf("Provisioning MariaDB switches the server's MariaDB repository to %s; with the package state set to latest it upgrades MariaDB to %s, so back up your databases first", target, target))
		case p.Cmp == "lt" && candidate != "" && candidate != server:
			// The server's repository already points at another series. Drop
			// any hold advice: unholding would allow that upgrade.
			r.Status = StatusWarn
			r.Message = fmt.Sprintf("%s installed; the server's MariaDB repository offers %s", upstreamVersion(p.Installed), upstreamVersion(p.Candidate))
			r.Details, upgradable = nil, true
			withheld = fmt.Sprintf("for a move from MariaDB %s to %s; back up your databases and upgrade deliberately", server, candidate)
		case target == "":
			withheld = unknownVersion(s)
		}
	}

	e.addPackageResult(s, r, p, upgradable, withheld)
}

func (e *evaluator) reboot() {
	r := Result{ID: "reboot", Title: "Reboot", Status: StatusOK, Message: "No reboot required"}

	if e.f.RebootRequired {
		r.Status, r.Message = StatusWarn, "Reboot required"
		if e.f.Uptime > 0 {
			r.Message += " · up " + humanDuration(e.f.Uptime)
		}
		r.Details = []string{"Reboot when convenient: connect with `trellis ssh <environment>` and run `sudo reboot`"}
	}

	e.add(r)
}

func (e *evaluator) unattendedUpgrades() {
	r := Result{ID: "unattended-upgrades", Title: "Unattended upgrades", Status: StatusWarn}
	enabled := e.f.UUSetting != "" && e.f.UUSetting != "0"
	const notApplied = "; Ubuntu security updates aren't applied automatically"

	switch {
	case !e.f.UUInstalled:
		r.Message = "not installed" + notApplied
		r.Details = []string{"Install it on the server: sudo apt-get install unattended-upgrades"}
	case !enabled:
		r.Message = "disabled (APT::Periodic::Unattended-Upgrade)" + notApplied
		r.Details = []string{"Enable it on the server: sudo dpkg-reconfigure -plow unattended-upgrades"}
	case e.f.UUTimer != "" && e.f.UUTimer != "enabled":
		r.Message = "apt-daily-upgrade.timer is " + e.f.UUTimer + notApplied
		r.Details = []string{"Enable it on the server: sudo systemctl enable --now apt-daily-upgrade.timer"}
	case e.f.UURan.IsZero():
		r.Status, r.Message = StatusUnknown, "enabled, but there's no record of it running yet"
	case e.since(e.f.UURan) > uuStaleAfter:
		r.Message = fmt.Sprintf("enabled, but last ran %s ago", humanDuration(e.since(e.f.UURan)))
	case e.f.SecurityPending > 0 && (e.f.AptUpdated.IsZero() || !e.f.UURan.After(e.f.AptUpdated)):
		// Updates published since the last run are applied on the next one.
		r.Status = StatusInfo
		r.Message = fmt.Sprintf("enabled · last ran %s ago · %d Ubuntu security %s pending for its next run", humanDuration(e.since(e.f.UURan)), e.f.SecurityPending, plural(e.f.SecurityPending, "update", "updates"))
	case e.f.SecurityPending > 0:
		r.Message = fmt.Sprintf("enabled, but %d Ubuntu security %s pending", e.f.SecurityPending, plural(e.f.SecurityPending, "update is", "updates are"))
		r.Details = []string{"It ran after the last package list update but didn't apply them; check: sudo tail -n 50 /var/log/unattended-upgrades/unattended-upgrades.log"}
		if e.f.SecurityApprox {
			r.Details = append(r.Details, "Count from `apt list --upgradable` (approximate)")
		}
	default:
		r.Status = StatusOK
		r.Message = fmt.Sprintf("enabled · last ran %s ago", humanDuration(e.since(e.f.UURan)))
		if e.f.SecurityPending < 0 {
			r.Message += " (pending security updates unknown)"
		}
	}

	e.add(r)
}

func humanDuration(d time.Duration) string {
	switch {
	case d < time.Hour:
		m := max(int(d.Minutes()), 0)
		return fmt.Sprintf("%d %s", m, plural(m, "minute", "minutes"))
	case d < 48*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%d %s", h, plural(h, "hour", "hours"))
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
