package doctor

import (
	"fmt"
	"slices"
	"strconv"
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

// ProjectVars are values resolved from the Trellis project for a host, the
// same way `trellis provision` would resolve them. Empty means unknown.
type ProjectVars struct {
	PHPVersion           string `json:"php_version"`
	MariaDBVersion       string `json:"mariadb_version"`
	NginxPackage         string `json:"nginx_package"`
	MariaDBServerPackage string `json:"mariadb_server_package"`
}

// PHPRelease is the latest upstream release of a PHP branch.
type PHPRelease struct {
	Version  string
	Date     time.Time
	Security bool
	// Supported is nil when php.net didn't say.
	Supported *bool
}

// PHPReleaseLookup returns the latest php.net release for a branch, or nil
// when unavailable (offline, network error, unknown branch).
type PHPReleaseLookup func(branch string) *PHPRelease

type HostReport struct {
	Results []Result `json:"results"`
	// Upgrades are services that `trellis provision` can bring up to date.
	Upgrades []Service `json:"upgrades,omitempty"`
}

func Evaluate(f *Facts, vars ProjectVars, php PHPReleaseLookup) HostReport {
	if php == nil {
		php = func(string) *PHPRelease { return nil }
	}

	// Ignore releases for a branch other than the one asked for.
	lookup := func(branch string) *PHPRelease {
		rel := php(branch)
		if rel == nil || Branch(rel.Version) != branch {
			return nil
		}
		return rel
	}

	e := &evaluator{f: f, vars: vars, php: lookup}

	e.osRelease()
	e.nginx()
	e.phpFPM()
	e.mariaDB()
	e.reboot()
	e.unattendedUpgrades()

	return e.report
}

type evaluator struct {
	f      *Facts
	vars   ProjectVars
	php    PHPReleaseLookup
	report HostReport
}

func (e *evaluator) add(r Result) {
	// List the apt source after any explanation.
	slices.SortStableFunc(r.Details, func(a, b string) int {
		return cmpBool(strings.HasPrefix(a, aptSourceDetail), strings.HasPrefix(b, aptSourceDetail))
	})
	e.report.Results = append(e.report.Results, r)
}

const aptSourceDetail = "apt source: "

func cmpBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	default:
		return -1
	}
}

func (e *evaluator) upgrade(s Service) {
	if !slices.Contains(e.report.Upgrades, s) {
		e.report.Upgrades = append(e.report.Upgrades, s)
	}
}

func (e *evaluator) since(t time.Time) time.Duration {
	return e.f.Now.Sub(t)
}

func (e *evaluator) aptListsStale() bool {
	return e.f.AptUpdated.IsZero() || e.since(e.f.AptUpdated) > aptListsStaleAfter
}

func (e *evaluator) aptListsAge() string {
	if e.f.AptUpdated.IsZero() {
		return "apt package lists age unknown"
	}
	return fmt.Sprintf("apt package lists last updated %s ago", humanDuration(e.since(e.f.AptUpdated)))
}

func (e *evaluator) osRelease() {
	r := Result{ID: "ubuntu-release", Title: "Ubuntu"}
	name := e.f.OSName
	if name == "" {
		name = strings.TrimSpace(e.f.OSID + " " + e.f.OSVersion)
	}

	switch {
	case name == "":
		r.Status = StatusSkip
		r.Message = "OS unknown (no /etc/os-release)"
	case e.f.OSID != "ubuntu":
		r.Status = StatusSkip
		r.Message = fmt.Sprintf("%s is not Ubuntu", name)
	case e.f.OSEOL.IsZero():
		r.Status = StatusSkip
		r.Message = fmt.Sprintf("%s (support dates unknown)", name)
	case !e.f.Now.Before(e.f.OSEOL):
		r.Status = StatusFail
		r.Message = fmt.Sprintf("%s reached end of standard support on %s", name, e.f.OSEOL.Format(time.DateOnly))
		r.Details = []string{"Ubuntu no longer publishes regular security updates for this release (Ubuntu Pro ESM aside)"}
	case e.f.OSEOL.Sub(e.f.Now) < osEOLWarnBefore:
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("%s standard support ends %s", name, e.f.OSEOL.Format(time.DateOnly))
	default:
		r.Status = StatusOK
		r.Message = fmt.Sprintf("%s · supported until %s", name, e.f.OSEOL.Format(time.DateOnly))
	}

	e.add(r)
}

// pkgResult evaluates installed vs candidate for a package. upgradable is
// true when provisioning with the package state set to latest would update it.
func (e *evaluator) pkgResult(id, name string) (r Result, upgradable bool) {
	r = Result{ID: id, Title: name}
	p := e.f.Packages[name]

	if p == nil || p.Installed == "" {
		r.Status = StatusSkip
		r.Message = "not installed"
		return r, false
	}

	installed := UpstreamVersion(p.Installed)

	switch {
	case p.AptError:
		r.Status = StatusUnknown
		r.Message = fmt.Sprintf("%s installed; apt-cache failed on the server, so available updates are unknown", installed)
		r.Details = append(r.Details, "Run `apt-cache policy` on the server to see the error; a broken apt source stops all updates")
		return r, false
	case p.Candidate == "":
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("%s installed, but no version is available from the server's apt sources", installed)
	case p.Cmp == "lt":
		from, to := displayVersions(p.Installed, p.Candidate)
		r.Status = StatusFail
		r.Message = fmt.Sprintf("%s → %s available", from, to)
		if p.Held {
			r.Details = append(r.Details, fmt.Sprintf("%s is held (apt-mark hold), so provisioning won't upgrade it; run `sudo apt-mark unhold %s` on the server first", name, name))
		} else {
			upgradable = true
		}
	case p.Cmp == "gt":
		from, to := displayVersions(p.Installed, p.Candidate)
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("%s installed is newer than %s available; the apt source may have changed", from, to)
	case p.Cmp == "eq" && p.Origin == "":
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("%s installed, but it isn't available from any of the server's apt sources, so it won't get updates", installed)
	case p.Cmp == "eq":
		if e.aptListsStale() {
			r.Status = StatusUnknown
			r.Message = fmt.Sprintf("%s (%s; can't confirm it's the latest)", installed, e.aptListsAge())
		} else {
			r.Status = StatusOK
			r.Message = installed
		}
	default:
		r.Status = StatusUnknown
		r.Message = fmt.Sprintf("%s installed (couldn't compare with %s)", installed, UpstreamVersion(p.Candidate))
	}

	if p.Origin != "" && r.Status != StatusOK {
		r.Details = append(r.Details, aptSourceDetail+p.Origin)
	}

	return r, upgradable
}

func (e *evaluator) nginx() {
	name := e.vars.NginxPackage
	if name == "" {
		name = "nginx"
	}

	r, upgradable := e.pkgResult("nginx", name)
	e.add(r)

	if upgradable {
		e.upgrade(ServiceNginx)
	}
}

// phpFPM evaluates the PHP-FPM branch serving sites, version drift against
// the project's php_version, upstream php.net releases, and leftovers.
func (e *evaluator) phpFPM() {
	branches := e.f.PHPBranches()
	if len(branches) == 0 {
		e.add(Result{ID: "php", Title: "PHP-FPM", Status: StatusSkip, Message: "not installed"})
		return
	}

	target := e.vars.PHPVersion
	serving := e.servingPHPBranch(branches, target)

	drift := target != "" && serving != target
	if drift {
		e.add(Result{
			ID:      "php-version",
			Title:   "PHP version",
			Status:  StatusWarn,
			Message: fmt.Sprintf("Server runs PHP %s but the project's php_version is %s", serving, target),
			Details: []string{
				fmt.Sprintf("Provisioning will switch sites to PHP %s", target),
				fmt.Sprintf("To stay on PHP %s, set php_version: \"%s\" in group_vars", serving, serving),
			},
		})
	}

	name := "php" + serving + "-fpm"
	rel := e.php(serving)
	r, upgradable := e.pkgResult("php", name)
	upgradable = e.applyPHPRelease(&r, name, rel) || upgradable

	if upgradable && (drift || target == "") {
		upgradable = false
		reason := "the PHP version mismatch is resolved"
		if target == "" {
			reason = "the project's php_version can be determined"
		}
		r.Details = append(r.Details, "No upgrade command suggested until "+reason)
	}

	e.add(r)

	if upgradable {
		e.upgrade(ServicePHP)
	}

	if rel != nil && rel.Supported != nil && !*rel.Supported {
		e.add(Result{
			ID:      "php-eol",
			Title:   "PHP " + serving,
			Status:  StatusWarn,
			Message: fmt.Sprintf("PHP %s is no longer supported upstream and gets no security fixes", serving),
		})
	}

	for _, b := range branches {
		// The serving branch is reported above; a drifted php_version is
		// covered by the version mismatch result.
		if b == serving || (drift && b == target) {
			continue
		}

		msg := "installed but " + e.f.PHPFPM[b]
		if e.f.PHPFPM[b] == "active" {
			msg = "also running; provisioning PHP stops and disables PHP-FPM versions other than php_version"
		}

		e.add(Result{
			ID:      "php-leftover-" + b,
			Title:   "php" + b + "-fpm",
			Status:  StatusInfo,
			Message: msg,
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

// applyPHPRelease compares the installed PHP with the latest php.net release
// for the branch. It covers the gap apt can't see: a release that isn't (yet)
// in the server's apt source. Returns true if provisioning may pick it up.
func (e *evaluator) applyPHPRelease(r *Result, name string, rel *PHPRelease) bool {
	p := e.f.Packages[name]
	if p == nil || p.Installed == "" || rel == nil {
		return false
	}

	installed := UpstreamVersion(p.Installed)
	if c, ok := compareVersions(installed, rel.Version); !ok || c >= 0 {
		return false
	}

	// php.net only publishes the release date, not the time.
	released := fmt.Sprintf("PHP %s was released on %s", rel.Version, rel.Date.Format(time.DateOnly))
	if rel.Security {
		released = fmt.Sprintf("PHP %s (security release) was released on %s", rel.Version, rel.Date.Format(time.DateOnly))
	}

	if p.AptError {
		// apt is broken, but the installed PHP is known to be outdated.
		r.Status = StatusFail
		r.Message = fmt.Sprintf("%s → %s released; apt-cache failed on the server", installed, rel.Version)
		r.Details = append(r.Details, released)
		return false
	}

	if p.Candidate != "" {
		if c, ok := compareVersions(UpstreamVersion(p.Candidate), rel.Version); ok && c >= 0 {
			// The apt source already has it; pkgResult reported the upgrade.
			r.Details = append(r.Details, released)
			return false
		}
	}

	upgradable := false

	switch {
	case !e.f.AptUpdated.IsZero() && e.f.AptUpdated.Before(rel.Date.Add(24*time.Hour)):
		// The server hasn't refreshed apt since the release. Provisioning
		// refreshes the apt cache, so it may pick the release up. Lists
		// updated on release day count as older.
		r.Status = StatusFail
		r.Message = fmt.Sprintf("%s → %s released", installed, rel.Version)
		r.Details = append(r.Details, released+"; the server's apt package lists predate it")
		switch {
		case !p.Held:
			upgradable = true
		case p.Cmp != "lt": // otherwise pkgResult already explained the hold
			r.Details = append(r.Details, fmt.Sprintf("%s is held (apt-mark hold), so provisioning won't upgrade it; run `sudo apt-mark unhold %s` on the server first", name, name))
		}
	case e.since(rel.Date) > phpSourceLagAfter:
		// Outdated PHP is a failure even when the apt source is the problem.
		r.Status = StatusFail
		r.Message = fmt.Sprintf("%s → %s released", installed, rel.Version)
		r.Details = append(r.Details, released+", but the server's apt source doesn't have it; that source may no longer be updated")
	default:
		if r.Status != StatusFail {
			r.Status = StatusWarn
		}
		r.Details = append(r.Details, released+"; it isn't available from the server's apt source yet")
	}

	if p.Origin != "" && !slices.Contains(r.Details, aptSourceDetail+p.Origin) {
		r.Details = append(r.Details, aptSourceDetail+p.Origin)
	}

	return upgradable
}

func (e *evaluator) mariaDB() {
	name := e.vars.MariaDBServerPackage
	if name == "" {
		name = "mariadb-server"
	}

	r, upgradable := e.pkgResult("mariadb", name)
	p := e.f.Packages[name]

	if p != nil && p.Installed != "" {
		serverBranch := Branch(p.Installed)
		candidateBranch := Branch(p.Candidate)
		target := e.vars.MariaDBVersion

		switch {
		case target != "" && serverBranch != "" && serverBranch != target:
			e.add(Result{
				ID:      "mariadb-version",
				Title:   "MariaDB version",
				Status:  StatusWarn,
				Message: fmt.Sprintf("Server runs MariaDB %s but the project's mariadb_version is %s", serverBranch, target),
				Details: []string{
					fmt.Sprintf("Provisioning MariaDB switches the server's MariaDB repository to %s; with the package state set to latest it upgrades MariaDB to %s, so back up your databases first", target, target),
					fmt.Sprintf("To stay on MariaDB %s, set mariadb_version: \"%s\" in group_vars", serverBranch, serverBranch),
				},
			})
			if upgradable {
				upgradable = false
				r.Details = append(r.Details, "No upgrade command suggested until the MariaDB version mismatch is resolved")
			}
		case p.Cmp == "lt" && candidateBranch != "" && candidateBranch != serverBranch:
			// The server's repository already points at another series.
			upgradable = false
			r.Status = StatusWarn
			r.Message = fmt.Sprintf("%s installed; the server's MariaDB repository offers %s", UpstreamVersion(p.Installed), UpstreamVersion(p.Candidate))
			r.Details = []string{fmt.Sprintf("Not suggesting an upgrade from MariaDB %s to %s; back up your databases and upgrade deliberately", serverBranch, candidateBranch)}
			if p.Origin != "" {
				r.Details = append(r.Details, aptSourceDetail+p.Origin)
			}
		case upgradable && target == "":
			upgradable = false
			r.Details = append(r.Details, "No upgrade command suggested because the project's mariadb_version couldn't be determined")
		}
	}

	e.add(r)

	if upgradable {
		e.upgrade(ServiceMariaDB)
	}
}

func (e *evaluator) reboot() {
	r := Result{ID: "reboot", Title: "Reboot"}
	uptime := ""
	if e.f.Uptime > 0 {
		uptime = fmt.Sprintf("running kernel %s, up %s", e.f.KernelRunning, humanDuration(e.f.Uptime))
	}

	const rebootHint = "Reboot when convenient: connect with `trellis ssh <environment>` and run `sudo reboot`"

	switch {
	case e.f.RebootRequired:
		r.Status = StatusWarn
		r.Message = "Reboot required"
		if len(e.f.RebootPackages) > 0 {
			r.Details = append(r.Details, "Updated: "+summarizeRebootPackages(e.f.RebootPackages))
		}
	case !e.f.RebootNotifier && kernelNewer(e.f.KernelLatest, e.f.KernelRunning):
		// Without update-notifier, /var/run/reboot-required is never
		// created, so fall back to comparing kernels.
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("Reboot required: kernel %s is installed", e.f.KernelLatest)
	default:
		r.Status = StatusOK
		r.Message = "No reboot required"
		uptime = ""
	}

	if uptime != "" {
		r.Details = append(r.Details, uptime)
	}
	if r.Status == StatusWarn {
		r.Details = append(r.Details, rebootHint)
	}

	e.add(r)
}

func (e *evaluator) unattendedUpgrades() {
	r := Result{ID: "unattended-upgrades", Title: "Unattended upgrades", Status: StatusWarn}
	enabled := e.f.UUSetting != "" && e.f.UUSetting != "0"

	switch {
	case !e.f.UUInstalled:
		r.Message = "not installed; Ubuntu security updates aren't applied automatically"
		r.Details = []string{"Install it on the server: sudo apt-get install unattended-upgrades"}
	case !enabled:
		r.Message = "disabled (APT::Periodic::Unattended-Upgrade); Ubuntu security updates aren't applied automatically"
		r.Details = []string{"Enable it on the server: sudo dpkg-reconfigure -plow unattended-upgrades"}
	case e.f.UUTimer != "" && e.f.UUTimer != "enabled":
		r.Message = fmt.Sprintf("apt-daily-upgrade.timer is %s; Ubuntu security updates aren't applied automatically", e.f.UUTimer)
		r.Details = []string{"Enable it on the server: sudo systemctl enable --now apt-daily-upgrade.timer"}
	case e.f.UURan.IsZero():
		r.Status = StatusUnknown
		r.Message = "enabled, but there's no record of it running yet"
	case e.since(e.f.UURan) > uuStaleAfter:
		r.Message = fmt.Sprintf("enabled, but last ran %s ago", humanDuration(e.since(e.f.UURan)))
	case e.f.SecurityPending > 0 && (e.f.AptUpdated.IsZero() || !e.f.UURan.After(e.f.AptUpdated)):
		// Updates published since the last run are applied on the next one.
		r.Status = StatusInfo
		r.Message = fmt.Sprintf("enabled · last ran %s ago · %d Ubuntu security %s pending for its next run", humanDuration(e.since(e.f.UURan)), e.f.SecurityPending, plural(e.f.SecurityPending, "update", "updates"))
	case e.f.SecurityPending > 0:
		r.Message = fmt.Sprintf("enabled, but %d Ubuntu security %s pending", e.f.SecurityPending, plural(e.f.SecurityPending, "update is", "updates are"))
		r.Details = append(r.Details, "It ran after the last package list update but didn't apply them; check: sudo tail -n 50 /var/log/unattended-upgrades/unattended-upgrades.log")
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

// kernelNewer reports whether kernel release a (e.g. "6.8.0-110-generic")
// is newer than b, comparing their numeric parts.
func kernelNewer(a, b string) bool {
	if a == "" || b == "" || a == b {
		return false
	}

	na, nb := numericParts(a), numericParts(b)
	for i := 0; i < len(na) && i < len(nb); i++ {
		if na[i] != nb[i] {
			return na[i] > nb[i]
		}
	}

	return len(na) > len(nb)
}

func numericParts(s string) []int {
	var parts []int
	for f := range strings.FieldsFuncSeq(s, func(r rune) bool { return r < '0' || r > '9' }) {
		n, err := strconv.Atoi(f)
		if err != nil {
			continue
		}
		parts = append(parts, n)
	}
	return parts
}

func humanDuration(d time.Duration) string {
	switch {
	case d < 0:
		return "0 minutes"
	case d < time.Hour:
		m := int(d.Minutes())
		return fmt.Sprintf("%d %s", m, plural(m, "minute", "minutes"))
	case d < 48*time.Hour:
		h := int(d.Hours())
		return fmt.Sprintf("%d %s", h, plural(h, "hour", "hours"))
	default:
		days := int(d.Hours() / 24)
		return fmt.Sprintf("%d days", days)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// summarizeRebootPackages lists the packages that need a reboot, collapsing
// kernel packages (often many, after months without a reboot).
func summarizeRebootPackages(pkgs []string) string {
	var others, kernels []string
	for _, p := range pkgs {
		if strings.HasPrefix(p, "linux-image-") || strings.HasPrefix(p, "linux-modules-") {
			kernels = append(kernels, p)
		} else {
			others = append(others, p)
		}
	}

	if len(kernels) <= 2 {
		return strings.Join(pkgs, ", ")
	}

	newest := kernels[0]
	for _, k := range kernels[1:] {
		if kernelNewer(strings.TrimPrefix(strings.TrimPrefix(k, "linux-image-"), "linux-modules-"),
			strings.TrimPrefix(strings.TrimPrefix(newest, "linux-image-"), "linux-modules-")) {
			newest = k
		}
	}

	return strings.Join(append(others, fmt.Sprintf("%d kernel packages (newest %s)", len(kernels), newest)), ", ")
}
