package doctor

import (
	_ "embed"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed probe.sh
var ProbeScript string

const (
	probeBegin   = "doctor_begin"
	probeEnd     = "doctor_end"
	probeVersion = "v=1"
)

var ErrProbeTruncated = errors.New("probe output was truncated (no end marker)")

type Package struct {
	Name      string
	Installed string // dpkg version, "" if not installed
	// Candidate is the dpkg version apt would install, "" if none. When an
	// installed package's source is gone, apt reports the installed version
	// as the candidate (with no Origin).
	Candidate string
	// Origin is the apt source of the candidate, e.g.
	// "https://nginx.org/packages/mainline/ubuntu noble/nginx"; "" if the
	// candidate isn't available from any source.
	Origin string
	Cmp    string // installed vs candidate on the server: "lt", "eq", "gt" or "" if unknown
	Held   bool
	// AptError is set when apt-cache failed on the server (e.g. a broken
	// source); only Installed is known then.
	AptError bool
}

// Facts are what the probe collected on one host.
type Facts struct {
	Now time.Time

	OSID      string
	OSVersion string
	OSName    string
	Codename  string
	OSEOL     time.Time // zero if unknown

	KernelRunning string
	KernelLatest  string
	Uptime        time.Duration

	RebootRequired bool
	RebootPackages []string
	RebootNotifier bool

	AptUpdated time.Time // last successful `apt-get update`, zero if unknown

	// Pending updates from update-notifier's apt-check; -1 if unknown.
	UpdatesPending  int
	SecurityPending int
	// SecurityApprox is true when SecurityPending was counted from
	// `apt list --upgradable` because apt-check isn't available.
	SecurityApprox bool

	UUInstalled bool
	UUSetting   string // APT::Periodic::Unattended-Upgrade
	UUTimer     string // systemctl is-enabled apt-daily-upgrade.timer
	UURan       time.Time

	// PHPFPM maps installed PHP-FPM branches (e.g. "8.3") to their systemd
	// state ("active", "inactive", ...).
	PHPFPM map[string]string

	Packages map[string]*Package
}

// ParseProbe parses the output of probe.sh. Lines outside the begin/end
// markers are ignored.
func ParseProbe(output string) (*Facts, error) {
	values := map[string]string{}
	begun, ended := false, false

	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimRight(line, "\r")

		if !begun {
			if fields := strings.Fields(line); len(fields) > 0 && fields[0] == probeBegin {
				if len(fields) < 2 || fields[1] != probeVersion {
					return nil, fmt.Errorf("unsupported probe output version: %q", line)
				}
				begun = true
			}
			continue
		}

		if line == probeEnd {
			ended = true
			break
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[key] = value
	}

	if !begun {
		return nil, errors.New("no probe output found")
	}
	if !ended {
		return nil, ErrProbeTruncated
	}

	return newFacts(values), nil
}

func newFacts(v map[string]string) *Facts {
	f := &Facts{
		Now:            unixTime(v["now"]),
		OSID:           v["os.id"],
		OSVersion:      v["os.version_id"],
		OSName:         v["os.name"],
		Codename:       v["os.codename"],
		KernelRunning:  v["kernel.running"],
		KernelLatest:   v["kernel.latest"],
		RebootRequired: v["reboot.required"] == "1",
		RebootNotifier: v["reboot.notifier"] == "1",
		UpdatesPending: -1,
		UUInstalled:    v["uu.installed"] == "1",
		UUSetting:      v["uu.setting"],
		UUTimer:        v["uu.timer"],
		UURan:          unixTime(v["uu.stamp"]),
		PHPFPM:         map[string]string{},
		Packages:       map[string]*Package{},
	}

	if f.Now.IsZero() {
		f.Now = time.Now()
	}

	if eol, err := time.Parse(time.DateOnly, v["os.eol"]); err == nil {
		f.OSEOL = eol
	}

	if secs, err := strconv.Atoi(v["uptime"]); err == nil {
		f.Uptime = time.Duration(secs) * time.Second
	}

	if pkgs := v["reboot.pkgs"]; pkgs != "" {
		f.RebootPackages = strings.Split(pkgs, ",")
	}

	f.AptUpdated = unixTime(v["apt.update_success"])
	if f.AptUpdated.IsZero() {
		f.AptUpdated = unixTime(v["apt.pkgcache"])
	}

	f.SecurityPending = -1
	if updates, security, ok := strings.Cut(v["apt.check"], ";"); ok {
		u, errU := strconv.Atoi(updates)
		s, errS := strconv.Atoi(security)
		if errU == nil && errS == nil {
			f.UpdatesPending = u
			f.SecurityPending = s
		}
	} else if n, err := strconv.Atoi(v["apt.upgradable_security"]); err == nil {
		f.SecurityPending = n
		f.SecurityApprox = true
	}

	for key, value := range v {
		if branch, ok := strings.CutPrefix(key, "php.fpm."); ok {
			f.PHPFPM[branch] = value
			continue
		}

		rest, ok := strings.CutPrefix(key, "pkg.")
		if !ok {
			continue
		}

		// Package names may contain dots (php8.3-fpm), fields don't.
		i := strings.LastIndex(rest, ".")
		if i <= 0 {
			continue
		}
		name, field := rest[:i], rest[i+1:]

		pkg, ok := f.Packages[name]
		if !ok {
			pkg = &Package{Name: name}
			f.Packages[name] = pkg
		}

		switch field {
		case "installed":
			pkg.Installed = noneToEmpty(value)
		case "candidate":
			pkg.Candidate = noneToEmpty(value)
		case "origin":
			pkg.Origin = value
		case "cmp":
			pkg.Cmp = value
		case "held":
			pkg.Held = value == "1"
		case "error":
			pkg.AptError = value == "1"
		}
	}

	return f
}

// PHPBranches returns installed PHP-FPM branches, sorted.
func (f *Facts) PHPBranches() []string {
	branches := make([]string, 0, len(f.PHPFPM))
	for b := range f.PHPFPM {
		branches = append(branches, b)
	}
	sort.Slice(branches, func(i, j int) bool {
		c, ok := compareVersions(branches[i], branches[j])
		if !ok {
			return branches[i] < branches[j]
		}
		return c < 0
	})
	return branches
}

func unixTime(s string) time.Time {
	secs, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || secs <= 0 {
		return time.Time{}
	}
	return time.Unix(secs, 0)
}

func noneToEmpty(s string) string {
	if s == "(none)" {
		return ""
	}
	return s
}
