package doctor

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

// healthyFacts is an up-to-date Ubuntu 24.04 Trellis server.
func healthyFacts() *hostFacts {
	return &hostFacts{
		Now:             testNow,
		OSID:            "ubuntu",
		OSVersion:       "24.04",
		OSName:          "Ubuntu 24.04.3 LTS",
		OSEOL:           time.Date(2029, 5, 31, 0, 0, 0, 0, time.UTC),
		Uptime:          10 * 24 * time.Hour,
		AptUpdated:      testNow.Add(-3 * time.Hour),
		SecurityPending: 0,
		UUInstalled:     true,
		UUSetting:       "1",
		UUTimer:         "enabled",
		UURan:           testNow.Add(-6 * time.Hour),
		PHPFPM:          map[string]string{"8.3": "active"},
		Packages: map[string]*aptPackage{
			"nginx":          {Installed: "1.31.9-1~noble", Candidate: "1.31.9-1~noble", Cmp: "eq", Origin: "https://nginx.org/packages/mainline/ubuntu noble/nginx"},
			"php8.3-fpm":     {Installed: "8.3.37-1+ubuntu24.04.1+deb.sury.org+1", Candidate: "8.3.37-1+ubuntu24.04.1+deb.sury.org+1", Cmp: "eq", Origin: "https://packages.sury.org/php noble/main"},
			"mariadb-server": {Installed: "1:10.11.15+maria~ubu2404", Candidate: "1:10.11.15+maria~ubu2404", Cmp: "eq", Origin: "https://mirror.rackspace.com/mariadb/repo/10.11/ubuntu noble/main"},
		},
	}
}

var testVars = projectVars{PHPVersion: "8.3", MariaDBVersion: "10.11"}

type releaseLookup = func(branch string) (*Release, error)

func phpRelease(version string, date time.Time, security bool) releaseLookup {
	return func(branch string) (*Release, error) {
		if branch != branchOf(version) {
			return nil, nil
		}
		return &Release{Version: version, Date: date, Security: security}, nil
	}
}

// evaluateWith runs evaluate with php as PHP's release lookup (offline if
// nil), and the playbook's default package names unless vars sets them.
func evaluateWith(t *testing.T, f *hostFacts, vars projectVars, php releaseLookup) HostResult {
	t.Helper()
	old := phpService.LatestRelease
	phpService.LatestRelease = php
	t.Cleanup(func() { phpService.LatestRelease = old })

	if vars.NginxPackage == "" {
		vars.NginxPackage = "nginx"
	}
	if vars.MariaDBServerPackage == "" {
		vars.MariaDBServerPackage = "mariadb-server"
	}

	return evaluate(f, vars, php == nil)
}

func findResult(t *testing.T, report HostResult, id string) Result {
	t.Helper()
	for _, r := range report.Results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no result with id %q in %+v", id, report.Results)
	return Result{}
}

func TestEvaluateHealthy(t *testing.T) {
	report := evaluateWith(t, healthyFacts(), testVars, phpRelease("8.3.37", testNow.AddDate(0, 0, -4), true))

	for _, r := range report.Results {
		if r.Status != StatusOK {
			t.Errorf("%s: expected ok, got %s: %s %v", r.ID, r.Status, r.Message, r.Details)
		}
	}
	if len(report.upgrades) != 0 || len(report.withheld) != 0 || report.staleLists {
		t.Errorf("unexpected report %+v", report)
	}
}

func TestEvaluateFixtures(t *testing.T) {
	cases := []struct {
		fixture  string
		vars     projectVars
		want     map[string]Status
		upgrades []*Service
	}{
		{
			fixture: "noble_outdated.txt",
			vars:    projectVars{PHPVersion: "8.4", MariaDBVersion: "10.11"},
			want: map[string]Status{
				"ubuntu-release": StatusOK, "nginx": StatusFail, "php": StatusFail, "mariadb": StatusFail,
				"reboot": StatusOK, "unattended-upgrades": StatusInfo,
			},
			upgrades: services,
		},
		{
			fixture: "resolute_fresh.txt",
			vars:    projectVars{PHPVersion: "8.5", MariaDBVersion: "11.8"},
			want: map[string]Status{
				"ubuntu-release": StatusOK, "nginx": StatusSkip, "php": StatusSkip, "mariadb": StatusSkip,
				"reboot": StatusOK, "unattended-upgrades": StatusUnknown,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.fixture, func(t *testing.T) {
			f, err := parseProbe(readFixture(t, tc.fixture))
			if err != nil {
				t.Fatal(err)
			}

			report := evaluateWith(t, f, tc.vars, nil)

			for id, want := range tc.want {
				if got := findResult(t, report, id).Status; got != want {
					t.Errorf("%s: expected %s, got %s", id, want, got)
				}
			}
			if !slices.Equal(report.upgrades, tc.upgrades) {
				t.Errorf("expected upgrades %v, got %v", tc.upgrades, report.upgrades)
			}
		})
	}
}

func TestEvaluate(t *testing.T) {
	stale := func(f *hostFacts) { f.AptUpdated = testNow.AddDate(0, 0, -5) }
	outdated := func(name, installed string) func(*hostFacts) {
		return func(f *hostFacts) { f.Packages[name].Installed, f.Packages[name].Cmp = installed, "lt" }
	}

	cases := []struct {
		name     string
		mutate   func(f *hostFacts)
		vars     *projectVars
		php      releaseLookup
		id       string
		status   Status
		contains string
		excludes string
		upgrades []*Service
		withheld []*Service
	}{
		// Packages
		{
			name: "package behind", mutate: outdated("nginx", "1.29.1-1~noble"),
			id: "nginx", status: StatusFail, contains: "1.29.1 → 1.31.9 available", upgrades: []*Service{nginxService},
		},
		{
			name: "packaging revision bump",
			mutate: func(f *hostFacts) {
				p := f.Packages["php8.3-fpm"]
				p.Installed, p.Candidate, p.Cmp = "8.3.37-2+0~deb.sury.org.1+ubuntu24.04", "8.3.37-3+debsuryorg24.04.1+1", "lt"
			},
			id: "php", status: StatusFail, contains: "8.3.37-2 → 8.3.37-3 available", upgrades: []*Service{phpService},
		},
		{
			name: "package behind but held",
			mutate: func(f *hostFacts) {
				outdated("nginx", "1.29.1-1~noble")(f)
				f.Packages["nginx"].Held = true
			},
			id: "nginx", status: StatusFail, contains: "apt-mark unhold nginx",
		},
		{
			name: "current but apt lists stale", mutate: stale,
			id: "nginx", status: StatusUnknown, contains: "last updated 5 days ago", upgrades: services,
		},
		{
			name: "apt lists age unknown", mutate: func(f *hostFacts) { f.AptUpdated = time.Time{} },
			id: "nginx", status: StatusUnknown, contains: "age unknown", upgrades: services,
		},
		{
			name: "stale apt lists and held",
			mutate: func(f *hostFacts) {
				stale(f)
				f.Packages["nginx"].Held = true
			},
			id: "nginx", status: StatusUnknown, contains: "apt-mark unhold nginx", upgrades: []*Service{phpService, mariaDBService},
		},
		{
			name: "behind with stale lists is still a failure",
			mutate: func(f *hostFacts) {
				stale(f)
				outdated("nginx", "1.29.1-1~noble")(f)
			},
			id: "nginx", status: StatusFail, upgrades: services,
		},
		{
			name:   "no candidate",
			mutate: func(f *hostFacts) { f.Packages["nginx"].Candidate, f.Packages["nginx"].Cmp = "", "" },
			id:     "nginx", status: StatusWarn, contains: "no version is available",
		},
		{
			name:   "installed newer than candidate",
			mutate: func(f *hostFacts) { f.Packages["nginx"].Candidate, f.Packages["nginx"].Cmp = "1.29.1-1~noble", "gt" },
			id:     "nginx", status: StatusWarn, contains: "may have changed",
		},
		{
			name:   "installed but no longer in any apt source",
			mutate: func(f *hostFacts) { f.Packages["nginx"].Origin = "" },
			id:     "nginx", status: StatusWarn, contains: "won't get updates",
		},
		{
			name:   "apt-cache failed",
			mutate: func(f *hostFacts) { f.Packages["nginx"] = &aptPackage{Installed: "1.29.1-1~noble", AptError: true} },
			id:     "nginx", status: StatusUnknown, contains: "apt-cache failed",
		},
		{
			name:   "unknown comparison",
			mutate: func(f *hostFacts) { f.Packages["nginx"].Cmp = "" },
			id:     "nginx", status: StatusUnknown,
		},
		{
			name:   "mariadb not installed",
			mutate: func(f *hostFacts) { delete(f.Packages, "mariadb-server") },
			id:     "mariadb", status: StatusSkip,
		},
		{
			name: "custom nginx package name",
			mutate: func(f *hostFacts) {
				f.Packages["nginx-custom"] = f.Packages["nginx"]
				delete(f.Packages, "nginx")
				outdated("nginx-custom", "1.29.1-1~noble")(f)
			},
			vars: &projectVars{PHPVersion: "8.3", MariaDBVersion: "10.11", NginxPackage: "nginx-custom"},
			id:   "nginx", status: StatusFail, upgrades: []*Service{nginxService},
		},

		// php.net
		{
			name: "php.net release already in the apt source", mutate: outdated("php8.3-fpm", "8.3.36-1"),
			php: phpRelease("8.3.37", testNow.AddDate(0, 0, -4), true),
			id:  "php", status: StatusFail, contains: "security release", upgrades: []*Service{phpService},
		},
		{
			name: "php.net release not yet in apt source",
			php:  phpRelease("8.3.38", testNow.AddDate(0, 0, -2), true),
			id:   "php", status: StatusWarn, contains: "isn't available from the server's apt source yet",
		},
		{
			name: "php apt source stopped updating",
			php:  phpRelease("8.3.38", testNow.AddDate(0, 0, -30), true),
			id:   "php", status: StatusFail, contains: "8.3.37 → 8.3.38 released",
		},
		{
			name: "php.net release with stale lists is included in the fix", mutate: stale,
			php: phpRelease("8.3.38", testNow.AddDate(0, 0, -1), true),
			id:  "php", status: StatusFail, contains: "8.3.37 → 8.3.38 released", excludes: "isn't available from the server's apt source",
			upgrades: services,
		},
		{
			name:   "apt-cache failed but php.net shows php outdated",
			mutate: func(f *hostFacts) { f.Packages["php8.3-fpm"] = &aptPackage{Installed: "8.3.30-1", AptError: true} },
			php:    phpRelease("8.3.37", testNow.AddDate(0, 0, -30), true),
			id:     "php", status: StatusFail, contains: "broken apt source",
		},
		{
			name:   "apt-cache failed and a recent php.net release",
			mutate: func(f *hostFacts) { f.Packages["php8.3-fpm"] = &aptPackage{Installed: "8.3.36-1", AptError: true} },
			php:    phpRelease("8.3.37", testNow.AddDate(0, 0, -2), true),
			id:     "php", status: StatusFail, excludes: "isn't available from the server's apt source",
		},
		{
			name: "php branch unsupported upstream",
			php: func(string) (*Release, error) {
				no := false
				return &Release{Version: "8.3.37", Date: testNow.AddDate(0, 0, -4), Supported: &no}, nil
			},
			id: "php-eol", status: StatusWarn,
		},
		{
			name: "php.net lookup failed",
			php:  func(string) (*Release, error) { return nil, errors.New("connection refused") },
			id:   "php", status: StatusUnknown, contains: "Couldn't check php.net's latest PHP 8.3 release: connection refused",
		},

		// Version switches
		{
			name: "php version drift withholds upgrade",
			mutate: func(f *hostFacts) {
				f.PHPFPM = map[string]string{"8.2": "active"}
				f.Packages["php8.2-fpm"] = &aptPackage{Installed: "8.2.30-1", Candidate: "8.2.36-1", Cmp: "lt", Origin: "x"}
			},
			id: "php", status: StatusFail, contains: "until the PHP version mismatch is resolved", withheld: []*Service{phpService},
		},
		{
			name: "php version unknown withholds upgrade", mutate: outdated("php8.3-fpm", "8.3.36-1"),
			vars: &projectVars{MariaDBVersion: "10.11"},
			id:   "php", status: StatusFail, contains: "php_version couldn't be determined", withheld: []*Service{phpService},
		},
		{
			name: "mariadb version drift withholds upgrade", mutate: outdated("mariadb-server", "1:10.11.14+maria~ubu2404"),
			vars: &projectVars{PHPVersion: "8.3", MariaDBVersion: "11.8"},
			id:   "mariadb-version", status: StatusWarn, contains: "back up your databases", withheld: []*Service{mariaDBService},
		},
		{
			name: "mariadb version unknown withholds upgrade", mutate: outdated("mariadb-server", "1:10.11.14+maria~ubu2404"),
			vars: &projectVars{PHPVersion: "8.3"},
			id:   "mariadb", status: StatusFail, contains: "mariadb_version couldn't be determined", withheld: []*Service{mariaDBService},
		},
		{
			name: "stale lists with unknown mariadb_version", mutate: stale,
			vars: &projectVars{PHPVersion: "8.3"},
			id:   "mariadb", status: StatusUnknown, contains: "mariadb_version couldn't be determined",
			upgrades: []*Service{nginxService, phpService}, withheld: []*Service{mariaDBService},
		},
		{
			name: "mariadb repository offers another series",
			mutate: func(f *hostFacts) {
				p := f.Packages["mariadb-server"]
				p.Candidate, p.Cmp, p.Held = "1:11.8.3+maria~ubu2404", "lt", true
			},
			id: "mariadb", status: StatusWarn, contains: "move from MariaDB 10.11 to 11.8", excludes: "unhold",
			withheld: []*Service{mariaDBService},
		},
		{
			name: "mariadb another series with unknown mariadb_version",
			mutate: func(f *hostFacts) {
				p := f.Packages["mariadb-server"]
				p.Candidate, p.Cmp, p.Held = "1:11.8.3+maria~ubu2404", "lt", true
			},
			vars: &projectVars{PHPVersion: "8.3"},
			id:   "mariadb", status: StatusWarn, contains: "move from MariaDB 10.11 to 11.8", excludes: "unhold",
			withheld: []*Service{mariaDBService},
		},
		{
			name: "current but on another mariadb series than the project's",
			vars: &projectVars{PHPVersion: "8.3", MariaDBVersion: "11.8"},
			id:   "mariadb-version", status: StatusWarn, withheld: []*Service{mariaDBService},
		},
		{
			name: "unknown project versions withhold",
			vars: &projectVars{},
			id:   "php", status: StatusOK, withheld: []*Service{phpService, mariaDBService},
		},
		{
			name: "mariadb patch upgrade", mutate: outdated("mariadb-server", "1:10.11.14+maria~ubu2404"),
			id: "mariadb", status: StatusFail, contains: "10.11.14 → 10.11.15 available", upgrades: []*Service{mariaDBService},
		},
		{
			name: "multiple php-fpm, project version active",
			mutate: func(f *hostFacts) {
				f.PHPFPM["8.4"] = "active"
				f.Packages["php8.4-fpm"] = &aptPackage{Installed: "8.4.20-1", Candidate: "8.4.20-1", Cmp: "eq", Origin: "x"}
			},
			id: "php", status: StatusOK, contains: "8.3.37",
		},

		// Server
		{
			name:   "reboot required",
			mutate: func(f *hostFacts) { f.RebootRequired = true },
			id:     "reboot", status: StatusWarn, contains: "up 10 days",
		},
		{
			name:   "reboot required hint",
			mutate: func(f *hostFacts) { f.RebootRequired = true },
			id:     "reboot", status: StatusWarn, contains: "run `sudo reboot` on the server",
		},
		{
			name:   "unattended-upgrades not installed",
			mutate: func(f *hostFacts) { f.UUInstalled = false },
			id:     "unattended-upgrades", status: StatusWarn, contains: "apt-get install unattended-upgrades",
		},
		{
			name:   "unattended-upgrades disabled",
			mutate: func(f *hostFacts) { f.UUSetting = "0" },
			id:     "unattended-upgrades", status: StatusWarn, contains: "dpkg-reconfigure",
		},
		{
			name:   "unattended-upgrades enabled with interval value",
			mutate: func(f *hostFacts) { f.UUSetting = "1d" },
			id:     "unattended-upgrades", status: StatusOK,
		},
		{
			name:   "unattended-upgrades timer disabled",
			mutate: func(f *hostFacts) { f.UUTimer = "disabled" },
			id:     "unattended-upgrades", status: StatusWarn, contains: "timer is disabled",
		},
		{
			name:   "unattended-upgrades never ran",
			mutate: func(f *hostFacts) { f.UURan = time.Time{} },
			id:     "unattended-upgrades", status: StatusUnknown,
		},
		{
			name:   "unattended-upgrades stale",
			mutate: func(f *hostFacts) { f.UURan = testNow.AddDate(0, 0, -4) },
			id:     "unattended-upgrades", status: StatusWarn, contains: "last ran 4 days ago",
		},
		{
			name:   "security updates pending for the next run",
			mutate: func(f *hostFacts) { f.SecurityPending = 3 },
			id:     "unattended-upgrades", status: StatusInfo, contains: "3 Ubuntu security updates pending for its next run",
		},
		{
			name: "unattended-upgrades not applying",
			mutate: func(f *hostFacts) {
				f.SecurityPending = 209
				f.UURan = testNow.Add(-time.Hour)
			},
			id: "unattended-upgrades", status: StatusWarn, contains: "209 Ubuntu security updates are pending",
		},
		{
			name: "ubuntu past end of support",
			mutate: func(f *hostFacts) {
				f.OSName, f.OSEOL = "Ubuntu 20.04.6 LTS", time.Date(2025, 5, 29, 0, 0, 0, 0, time.UTC)
			},
			id: "ubuntu-release", status: StatusFail,
		},
		{
			name:   "ubuntu near end of support",
			mutate: func(f *hostFacts) { f.OSEOL = testNow.AddDate(0, 2, 0) },
			id:     "ubuntu-release", status: StatusWarn,
		},
		{
			name:   "ubuntu support dates unknown",
			mutate: func(f *hostFacts) { f.OSEOL = time.Time{} },
			id:     "ubuntu-release", status: StatusSkip,
		},
		{
			name:   "os unknown",
			mutate: func(f *hostFacts) { f.OSID, f.OSName, f.OSVersion = "", "", "" },
			id:     "ubuntu-release", status: StatusSkip, contains: "OS unknown",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := healthyFacts()
			if tc.mutate != nil {
				tc.mutate(f)
			}
			vars := testVars
			if tc.vars != nil {
				vars = *tc.vars
			}

			report := evaluateWith(t, f, vars, tc.php)
			r := findResult(t, report, tc.id)
			text := r.Message + "\n" + strings.Join(r.Details, "\n")

			if r.Status != tc.status {
				t.Errorf("expected status %s, got %s: %s", tc.status, r.Status, text)
			}
			if tc.contains != "" && !strings.Contains(text, tc.contains) {
				t.Errorf("expected %q in:\n%s", tc.contains, text)
			}
			if tc.excludes != "" && strings.Contains(text, tc.excludes) {
				t.Errorf("didn't expect %q in:\n%s", tc.excludes, text)
			}
			if !slices.Equal(report.upgrades, tc.upgrades) {
				t.Errorf("expected upgrades %v, got %v", tc.upgrades, report.upgrades)
			}
			if !slices.Equal(report.withheld, tc.withheld) {
				t.Errorf("expected withheld %v, got %v", tc.withheld, report.withheld)
			}
		})
	}
}
