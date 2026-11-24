package doctor

import (
	"slices"
	"strings"
	"testing"
	"time"
)

var testNow = time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)

// healthyFacts is an up-to-date Ubuntu 24.04 Trellis server.
func healthyFacts() *Facts {
	return &Facts{
		Now:             testNow,
		OSID:            "ubuntu",
		OSVersion:       "24.04",
		OSName:          "Ubuntu 24.04.3 LTS",
		Codename:        "noble",
		OSEOL:           time.Date(2029, 5, 31, 0, 0, 0, 0, time.UTC),
		KernelRunning:   "6.8.0-110-generic",
		KernelLatest:    "6.8.0-110-generic",
		Uptime:          10 * 24 * time.Hour,
		RebootNotifier:  true,
		AptUpdated:      testNow.Add(-3 * time.Hour),
		UpdatesPending:  0,
		SecurityPending: 0,
		UUInstalled:     true,
		UUSetting:       "1",
		UUTimer:         "enabled",
		UURan:           testNow.Add(-6 * time.Hour),
		PHPFPM:          map[string]string{"8.3": "active"},
		Packages: map[string]*Package{
			"nginx":          {Name: "nginx", Installed: "1.31.9-1~noble", Candidate: "1.31.9-1~noble", Cmp: "eq", Origin: "https://nginx.org/packages/mainline/ubuntu noble/nginx"},
			"php8.3-fpm":     {Name: "php8.3-fpm", Installed: "8.3.37-1+ubuntu24.04.1+deb.sury.org+1", Candidate: "8.3.37-1+ubuntu24.04.1+deb.sury.org+1", Cmp: "eq", Origin: "https://packages.sury.org/php noble/main"},
			"mariadb-server": {Name: "mariadb-server", Installed: "1:10.11.15+maria~ubu2404", Candidate: "1:10.11.15+maria~ubu2404", Cmp: "eq", Origin: "https://mirror.rackspace.com/mariadb/repo/10.11/ubuntu noble/main"},
		},
	}
}

var testVars = ProjectVars{PHPVersion: "8.3", MariaDBVersion: "10.11"}

func phpRelease(version string, date time.Time, security bool) PHPReleaseLookup {
	return func(branch string) *PHPRelease {
		if branch != Branch(version) {
			return nil
		}
		return &PHPRelease{Version: version, Date: date, Security: security}
	}
}

func findResult(t *testing.T, report HostReport, id string) Result {
	t.Helper()
	for _, r := range report.Results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no result with id %q in %+v", id, report.Results)
	return Result{}
}

func hasResultPrefix(report HostReport, prefix string) bool {
	return slices.ContainsFunc(report.Results, func(r Result) bool { return strings.HasPrefix(r.ID, prefix) })
}

func TestEvaluateHealthy(t *testing.T) {
	report := Evaluate(healthyFacts(), testVars, phpRelease("8.3.37", testNow.AddDate(0, 0, -4), true))

	for _, r := range report.Results {
		if r.Status != StatusOK && (r.Status != StatusInfo || r.ID != "ubuntu-newer") {
			t.Errorf("%s: expected ok, got %s: %s %v", r.ID, r.Status, r.Message, r.Details)
		}
	}
	if len(report.Upgrades) != 0 {
		t.Errorf("expected no upgrades, got %v", report.Upgrades)
	}
}

func TestEvaluateFixture(t *testing.T) {
	f, err := ParseProbe(readFixture(t, "noble_outdated.txt"))
	if err != nil {
		t.Fatal(err)
	}

	report := Evaluate(f, ProjectVars{PHPVersion: "8.4", MariaDBVersion: "10.11"}, nil)

	for id, want := range map[string]Status{
		"ubuntu-release":      StatusOK,
		"nginx":               StatusFail,
		"php":                 StatusFail,
		"mariadb":             StatusFail,
		"reboot":              StatusOK,
		"unattended-upgrades": StatusInfo,
		"php-leftover-8.3":    StatusInfo,
	} {
		if got := findResult(t, report, id).Status; got != want {
			t.Errorf("%s: expected %s, got %s", id, want, got)
		}
	}

	if got := findResult(t, report, "php").Message; got != "8.4.18 → 8.4.20 available" {
		t.Errorf("unexpected php message %q", got)
	}
	if !slices.Equal(report.Upgrades, []Service{ServiceNginx, ServicePHP, ServiceMariaDB}) {
		t.Errorf("unexpected upgrades %v", report.Upgrades)
	}
}

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(f *Facts)
		vars     *ProjectVars
		php      PHPReleaseLookup
		id       string
		status   Status
		contains string
		upgrades []Service
	}{
		{
			name: "package behind",
			mutate: func(f *Facts) {
				p := f.Packages["nginx"]
				p.Installed, p.Cmp = "1.29.1-1~noble", "lt"
			},
			id: "nginx", status: StatusFail, contains: "1.29.1 → 1.31.9 available",
			upgrades: []Service{ServiceNginx},
		},
		{
			name: "package behind but held",
			mutate: func(f *Facts) {
				p := f.Packages["nginx"]
				p.Installed, p.Cmp, p.Held = "1.29.1-1~noble", "lt", true
			},
			id: "nginx", status: StatusFail, contains: "apt-mark unhold nginx",
		},
		{
			name:   "current but apt lists stale",
			mutate: func(f *Facts) { f.AptUpdated = testNow.AddDate(0, 0, -5) },
			id:     "nginx", status: StatusUnknown, contains: "5 days ago",
		},
		{
			name: "behind with stale lists is still a failure",
			mutate: func(f *Facts) {
				f.AptUpdated = testNow.AddDate(0, 0, -5)
				p := f.Packages["nginx"]
				p.Installed, p.Cmp = "1.29.1-1~noble", "lt"
			},
			id: "nginx", status: StatusFail, upgrades: []Service{ServiceNginx},
		},
		{
			name:   "no candidate",
			mutate: func(f *Facts) { f.Packages["nginx"].Candidate, f.Packages["nginx"].Cmp = "", "" },
			id:     "nginx", status: StatusWarn, contains: "no version is available",
		},
		{
			name: "installed newer than candidate",
			mutate: func(f *Facts) {
				p := f.Packages["nginx"]
				p.Candidate, p.Cmp = "1.29.1-1~noble", "gt"
			},
			id: "nginx", status: StatusWarn, contains: "may have changed",
		},
		{
			name:   "installed but no longer in any apt source",
			mutate: func(f *Facts) { f.Packages["nginx"].Origin = "" },
			id:     "nginx", status: StatusWarn, contains: "won't get updates",
		},
		{
			name: "apt-cache failed",
			mutate: func(f *Facts) {
				f.Packages["nginx"] = &Package{Name: "nginx", Installed: "1.29.1-1~noble", AptError: true}
			},
			id: "nginx", status: StatusUnknown, contains: "apt-cache failed",
		},
		{
			name:   "unknown comparison",
			mutate: func(f *Facts) { f.Packages["nginx"].Cmp = "" },
			id:     "nginx", status: StatusUnknown,
		},
		{
			name:   "mariadb not installed",
			mutate: func(f *Facts) { delete(f.Packages, "mariadb-server") },
			id:     "mariadb", status: StatusSkip,
		},
		{
			name: "custom nginx package name",
			mutate: func(f *Facts) {
				p := f.Packages["nginx"]
				delete(f.Packages, "nginx")
				p.Name, p.Installed, p.Cmp = "nginx-custom", "1.29.1-1~noble", "lt"
				f.Packages["nginx-custom"] = p
			},
			vars: &ProjectVars{PHPVersion: "8.3", NginxPackage: "nginx-custom"},
			id:   "nginx", status: StatusFail, upgrades: []Service{ServiceNginx},
		},
		{
			name: "php.net release not yet in apt source",
			php:  phpRelease("8.3.38", testNow.AddDate(0, 0, -2), true),
			id:   "php", status: StatusWarn, contains: "isn't available from the server's apt source yet",
		},
		{
			name: "php.net release the same day the apt lists were updated",
			php:  phpRelease("8.3.38", testNow.Truncate(24*time.Hour), true),
			id:   "php", status: StatusFail, contains: "predate",
			upgrades: []Service{ServicePHP},
		},
		{
			name:   "php.net release newer than server's apt lists",
			mutate: func(f *Facts) { f.AptUpdated = testNow.AddDate(0, 0, -2) },
			php:    phpRelease("8.3.38", testNow.AddDate(0, 0, -1), true),
			id:     "php", status: StatusFail, contains: "8.3.37 → 8.3.38 released",
			upgrades: []Service{ServicePHP},
		},
		{
			name: "php apt source stopped updating",
			mutate: func(f *Facts) {
				p := f.Packages["php8.3-fpm"]
				p.Installed, p.Candidate = "8.3.30-3+ubuntu24.04.1+deb.sury.org+1", "8.3.30-3+ubuntu24.04.1+deb.sury.org+1"
				p.Origin = "https://ppa.launchpadcontent.net/ondrej/php/ubuntu noble/main"
			},
			php: phpRelease("8.3.37", testNow.AddDate(0, 0, -30), true),
			id:  "php", status: StatusFail, contains: "ppa.launchpadcontent.net",
		},
		{
			name: "php.net release already in candidate",
			mutate: func(f *Facts) {
				p := f.Packages["php8.3-fpm"]
				p.Installed, p.Cmp = "8.3.36-1+ubuntu24.04.1+deb.sury.org+1", "lt"
			},
			php: phpRelease("8.3.37", testNow.AddDate(0, 0, -4), true),
			id:  "php", status: StatusFail, contains: "security release",
			upgrades: []Service{ServicePHP},
		},
		{
			name: "php version drift withholds upgrade",
			mutate: func(f *Facts) {
				f.PHPFPM = map[string]string{"8.2": "active"}
				f.Packages["php8.2-fpm"] = &Package{Name: "php8.2-fpm", Installed: "8.2.30-1", Candidate: "8.2.36-1", Cmp: "lt"}
			},
			id: "php-version", status: StatusWarn, contains: "Server runs PHP 8.2 but the project's php_version is 8.3",
		},
		{
			name: "multiple php-fpm, project version active",
			mutate: func(f *Facts) {
				f.PHPFPM["8.4"] = "active"
				f.Packages["php8.4-fpm"] = &Package{Name: "php8.4-fpm", Installed: "8.4.20-1", Candidate: "8.4.20-1", Cmp: "eq"}
			},
			id: "php-leftover-8.4", status: StatusInfo, contains: "also running",
		},
		{
			name: "php branch unsupported upstream",
			php: func(string) *PHPRelease {
				no := false
				return &PHPRelease{Version: "8.3.37", Date: testNow.AddDate(0, 0, -4), Supported: &no}
			},
			id: "php-eol", status: StatusWarn,
		},
		{
			name:   "mariadb major version drift withholds upgrade",
			mutate: func(f *Facts) { f.Packages["mariadb-server"].Cmp = "lt" },
			vars:   &ProjectVars{PHPVersion: "8.3", MariaDBVersion: "11.8"},
			id:     "mariadb-version", status: StatusWarn, contains: "back up your databases",
		},
		{
			name: "reboot required",
			mutate: func(f *Facts) {
				f.RebootRequired = true
				f.RebootPackages = []string{"linux-base"}
			},
			id: "reboot", status: StatusWarn, contains: "trellis ssh <environment>",
		},
		{
			name: "reboot required with unknown uptime",
			mutate: func(f *Facts) {
				f.RebootRequired = true
				f.Uptime = 0
			},
			id: "reboot", status: StatusWarn, contains: "sudo reboot",
		},
		{
			name: "newer kernel without update-notifier",
			mutate: func(f *Facts) {
				f.RebootNotifier = false
				f.KernelLatest = "6.8.0-111-generic"
			},
			id: "reboot", status: StatusWarn, contains: "6.8.0-111-generic",
		},
		{
			name: "newer kernel ignored when update-notifier is present",
			mutate: func(f *Facts) {
				f.KernelLatest = "6.8.0-111-generic"
			},
			id: "reboot", status: StatusOK,
		},
		{
			name:   "unattended-upgrades disabled",
			mutate: func(f *Facts) { f.UUSetting = "0" },
			id:     "unattended-upgrades", status: StatusWarn, contains: "disabled",
		},
		{
			name:   "unattended-upgrades enabled with interval value",
			mutate: func(f *Facts) { f.UUSetting = "1d" },
			id:     "unattended-upgrades", status: StatusOK,
		},
		{
			name:   "unattended-upgrades stale",
			mutate: func(f *Facts) { f.UURan = testNow.AddDate(0, 0, -4) },
			id:     "unattended-upgrades", status: StatusWarn, contains: "last ran 4 days ago",
		},
		{
			name:   "security updates pending for the next run",
			mutate: func(f *Facts) { f.SecurityPending = 3 },
			id:     "unattended-upgrades", status: StatusInfo, contains: "3 Ubuntu security updates pending for its next run",
		},
		{
			name: "unattended-upgrades not applying",
			mutate: func(f *Facts) {
				f.SecurityPending = 209
				f.UURan = testNow.Add(-time.Hour)
			},
			id: "unattended-upgrades", status: StatusWarn, contains: "209 Ubuntu security updates are pending",
		},
		{
			name:   "unattended-upgrades not installed",
			mutate: func(f *Facts) { f.UUInstalled = false },
			id:     "unattended-upgrades", status: StatusWarn, contains: "not installed",
		},
		{
			name:   "unattended-upgrades timer disabled",
			mutate: func(f *Facts) { f.UUTimer = "disabled" },
			id:     "unattended-upgrades", status: StatusWarn, contains: "timer is disabled",
		},
		{
			name:   "unattended-upgrades never ran",
			mutate: func(f *Facts) { f.UURan = time.Time{} },
			id:     "unattended-upgrades", status: StatusUnknown,
		},
		{
			name:   "os unknown",
			mutate: func(f *Facts) { f.OSID, f.OSName, f.OSVersion = "", "", "" },
			id:     "ubuntu-release", status: StatusSkip, contains: "OS unknown",
		},
		{
			name:   "mariadb version unknown withholds upgrade",
			mutate: func(f *Facts) { f.Packages["mariadb-server"].Cmp = "lt" },
			vars:   &ProjectVars{PHPVersion: "8.3"},
			id:     "mariadb", status: StatusFail, contains: "mariadb_version couldn't be determined",
		},
		{
			name: "mariadb repository offers another series",
			mutate: func(f *Facts) {
				p := f.Packages["mariadb-server"]
				p.Candidate, p.Cmp = "1:11.8.3+maria~ubu2404", "lt"
			},
			id: "mariadb", status: StatusWarn, contains: "Not suggesting an upgrade from MariaDB 10.11 to 11.8",
		},
		{
			name: "mariadb patch upgrade",
			mutate: func(f *Facts) {
				p := f.Packages["mariadb-server"]
				p.Installed, p.Cmp = "1:10.11.14+maria~ubu2404", "lt"
			},
			id: "mariadb", status: StatusFail, contains: "10.11.14 → 10.11.15 available",
			upgrades: []Service{ServiceMariaDB},
		},
		{
			name: "php version unknown withholds upgrade",
			mutate: func(f *Facts) {
				p := f.Packages["php8.3-fpm"]
				p.Installed, p.Cmp = "8.3.36-1+ubuntu24.04.1+deb.sury.org+1", "lt"
			},
			vars: &ProjectVars{MariaDBVersion: "10.11"},
			id:   "php", status: StatusFail, contains: "php_version can be determined",
		},
		{
			name: "php.net release for another branch is ignored",
			php: func(string) *PHPRelease {
				return &PHPRelease{Version: "8.4.21", Date: testNow.AddDate(0, 0, -1)}
			},
			id: "php", status: StatusOK,
		},
		{
			name: "held php with release newer than apt lists",
			mutate: func(f *Facts) {
				f.AptUpdated = testNow.AddDate(0, 0, -2)
				f.Packages["php8.3-fpm"].Held = true
			},
			php: phpRelease("8.3.38", testNow.AddDate(0, 0, -1), true),
			id:  "php", status: StatusFail, contains: "apt-mark unhold php8.3-fpm",
		},
		{
			name:   "stale lists and php.net release newer than lists",
			mutate: func(f *Facts) { f.AptUpdated = testNow.AddDate(0, 0, -5) },
			php:    phpRelease("8.3.38", testNow.AddDate(0, 0, -1), true),
			id:     "php", status: StatusFail, contains: "predate",
			upgrades: []Service{ServicePHP},
		},
		{
			name: "drifted php_version installed but inactive isn't a leftover",
			mutate: func(f *Facts) {
				f.PHPFPM = map[string]string{"8.3": "inactive", "8.4": "active"}
				f.Packages["php8.4-fpm"] = &Package{Name: "php8.4-fpm", Installed: "8.4.20-1", Candidate: "8.4.20-1", Cmp: "eq", Origin: "x"}
			},
			id: "php-version", status: StatusWarn, contains: "Server runs PHP 8.4",
		},
		{
			name: "held php behind apt and php.net explains the hold once",
			mutate: func(f *Facts) {
				f.AptUpdated = testNow.AddDate(0, 0, -2)
				p := f.Packages["php8.3-fpm"]
				p.Installed, p.Cmp, p.Held = "8.3.35-1+ubuntu24.04.1+deb.sury.org+1", "lt", true
			},
			php: phpRelease("8.3.38", testNow.AddDate(0, 0, -1), true),
			id:  "php", status: StatusFail, contains: "apt-mark unhold php8.3-fpm",
		},
		{
			name: "held mariadb offered in another series",
			mutate: func(f *Facts) {
				p := f.Packages["mariadb-server"]
				p.Candidate, p.Cmp, p.Held = "1:11.8.3+maria~ubu2404", "lt", true
			},
			id: "mariadb", status: StatusWarn, contains: "Not suggesting an upgrade",
		},
		{
			name: "apt-cache failed but php.net shows php outdated",
			mutate: func(f *Facts) {
				f.Packages["php8.3-fpm"] = &Package{Name: "php8.3-fpm", Installed: "8.3.30-1", AptError: true}
			},
			php: phpRelease("8.3.37", testNow.AddDate(0, 0, -30), true),
			id:  "php", status: StatusFail, contains: "apt-cache failed",
		},
		{
			name: "security pending with unknown apt list age",
			mutate: func(f *Facts) {
				f.SecurityPending = 5
				f.AptUpdated = time.Time{}
			},
			id: "unattended-upgrades", status: StatusInfo,
		},
		{
			name:   "ubuntu past end of support",
			mutate: func(f *Facts) { f.OSName, f.OSEOL = "Ubuntu 20.04.6 LTS", time.Date(2025, 5, 29, 0, 0, 0, 0, time.UTC) },
			id:     "ubuntu-release", status: StatusFail,
		},
		{
			name:   "ubuntu near end of support",
			mutate: func(f *Facts) { f.OSEOL = testNow.AddDate(0, 2, 0) },
			id:     "ubuntu-release", status: StatusWarn,
		},
		{
			name:   "ubuntu support dates unknown",
			mutate: func(f *Facts) { f.OSEOL = time.Time{} },
			id:     "ubuntu-release", status: StatusSkip,
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

			report := Evaluate(f, vars, tc.php)
			r := findResult(t, report, tc.id)

			if r.Status != tc.status {
				t.Errorf("expected status %s, got %s: %s %v", tc.status, r.Status, r.Message, r.Details)
			}
			if tc.contains != "" {
				text := r.Message + "\n" + strings.Join(r.Details, "\n")
				if !strings.Contains(text, tc.contains) {
					t.Errorf("expected %q in:\n%s", tc.contains, text)
				}
			}
			if !slices.Equal(report.Upgrades, tc.upgrades) {
				t.Errorf("expected upgrades %v, got %v", tc.upgrades, report.Upgrades)
			}
		})
	}
}

func TestEvaluatePHPDriftReportsServingBranch(t *testing.T) {
	f := healthyFacts()
	f.PHPFPM = map[string]string{"8.2": "active"}
	f.Packages["php8.2-fpm"] = &Package{Name: "php8.2-fpm", Installed: "8.2.30-1", Candidate: "8.2.36-1", Cmp: "lt"}

	report := Evaluate(f, testVars, nil)
	r := findResult(t, report, "php")

	if r.Title != "php8.2-fpm" || r.Status != StatusFail {
		t.Errorf("expected failing php8.2-fpm, got %+v", r)
	}
	if !strings.Contains(strings.Join(r.Details, "\n"), "No upgrade command suggested") {
		t.Errorf("expected withheld upgrade note, got %v", r.Details)
	}
	if hasResultPrefix(report, "php-leftover") {
		t.Error("serving branch reported as leftover")
	}
}

func TestEvaluateNoLeftoverForDriftTarget(t *testing.T) {
	f := healthyFacts()
	f.PHPFPM = map[string]string{"8.3": "inactive", "8.4": "active"}
	f.Packages["php8.4-fpm"] = &Package{Name: "php8.4-fpm", Installed: "8.4.20-1", Candidate: "8.4.20-1", Cmp: "eq", Origin: "x"}

	if report := Evaluate(f, testVars, nil); hasResultPrefix(report, "php-leftover") {
		t.Error("project php_version reported as a leftover")
	}
}

func TestEvaluateMultipleActivePHPWithoutProjectVersion(t *testing.T) {
	f := healthyFacts()
	f.PHPFPM = map[string]string{"8.3": "active", "8.4": "active"}
	f.Packages["php8.4-fpm"] = &Package{Name: "php8.4-fpm", Installed: "8.4.20-1", Candidate: "8.4.20-1", Cmp: "eq", Origin: "x"}

	report := Evaluate(f, ProjectVars{}, nil)

	if r := findResult(t, report, "php"); r.Title != "php8.4-fpm" {
		t.Errorf("expected highest active branch, got %s", r.Title)
	}
	if r := findResult(t, report, "php-leftover-8.3"); !strings.Contains(r.Message, "also running") {
		t.Errorf("unexpected leftover message %q", r.Message)
	}
}

func TestEvaluateHeldDetailNotDuplicated(t *testing.T) {
	f := healthyFacts()
	f.AptUpdated = testNow.AddDate(0, 0, -2)
	p := f.Packages["php8.3-fpm"]
	p.Installed, p.Cmp, p.Held = "8.3.35-1+ubuntu24.04.1+deb.sury.org+1", "lt", true

	r := findResult(t, Evaluate(f, testVars, phpRelease("8.3.38", testNow.AddDate(0, 0, -1), true)), "php")

	if n := strings.Count(strings.Join(r.Details, "\n"), "apt-mark unhold"); n != 1 {
		t.Errorf("expected the hold explained once, got %d times: %v", n, r.Details)
	}
}

func TestEvaluateHeldMariaDBSeriesHasNoUnholdAdvice(t *testing.T) {
	f := healthyFacts()
	p := f.Packages["mariadb-server"]
	p.Candidate, p.Cmp, p.Held = "1:11.8.3+maria~ubu2404", "lt", true

	r := findResult(t, Evaluate(f, testVars, nil), "mariadb")

	if strings.Contains(strings.Join(r.Details, "\n"), "unhold") {
		t.Errorf("didn't expect unhold advice before a series upgrade: %v", r.Details)
	}
}

func TestEvaluateListsAptSourceLast(t *testing.T) {
	f := healthyFacts()
	p := f.Packages["php8.3-fpm"]
	p.Installed = "8.3.36-1+ubuntu24.04.1+deb.sury.org+1"
	p.Candidate = p.Installed

	r := findResult(t, Evaluate(f, testVars, phpRelease("8.3.37", testNow.AddDate(0, 0, -3), true)), "php")

	if len(r.Details) < 2 || !strings.HasPrefix(r.Details[len(r.Details)-1], "apt source: ") {
		t.Errorf("expected the apt source last, got %v", r.Details)
	}
}

func TestEvaluateFreshResolute(t *testing.T) {
	f, err := ParseProbe(readFixture(t, "resolute_fresh.txt"))
	if err != nil {
		t.Fatal(err)
	}

	report := Evaluate(f, ProjectVars{PHPVersion: "8.5", MariaDBVersion: "11.8"}, nil)

	for id, want := range map[string]Status{
		"ubuntu-release":      StatusOK,
		"nginx":               StatusSkip,
		"php":                 StatusSkip,
		"mariadb":             StatusSkip,
		"reboot":              StatusOK,
		"unattended-upgrades": StatusUnknown,
	} {
		if got := findResult(t, report, id).Status; got != want {
			t.Errorf("%s: expected %s, got %s", id, want, got)
		}
	}
}

func TestEvaluateNewerUbuntu(t *testing.T) {
	old := supportedUbuntu
	supportedUbuntu = func() []string { return []string{"22.04", "26.04", "24.04"} }
	defer func() { supportedUbuntu = old }()

	report := Evaluate(healthyFacts(), testVars, nil)
	r := findResult(t, report, "ubuntu-newer")
	if r.Status != StatusInfo || !strings.Contains(r.Message, "26.04 is the newest LTS release Trellis supports") || r.Title != "Newer Ubuntu" {
		t.Errorf("unexpected result %+v", r)
	}

	f := healthyFacts()
	f.OSVersion = "26.04"
	if report := Evaluate(f, testVars, nil); hasResultPrefix(report, "ubuntu-newer") {
		t.Error("didn't expect a newer release on the newest Ubuntu")
	}

	f.OSID = "debian"
	if report := Evaluate(f, testVars, nil); hasResultPrefix(report, "ubuntu-newer") {
		t.Error("didn't expect a newer Ubuntu release on Debian")
	}
}

func TestNewestSupportedUbuntu(t *testing.T) {
	if newest := newestSupportedUbuntu(); newest == "" {
		t.Error("expected trellis-cli to support at least one Ubuntu release")
	}

	old := supportedUbuntu
	supportedUbuntu = func() []string { return []string{"bogus", "24.04", "26.04", "22.04"} }
	defer func() { supportedUbuntu = old }()

	if newest := newestSupportedUbuntu(); newest != "26.04" {
		t.Errorf("expected 26.04, got %q", newest)
	}
}
