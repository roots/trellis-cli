package doctor

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestParseProbeFixture(t *testing.T) {
	f, err := ParseProbe(readFixture(t, "noble_outdated.txt"))
	if err != nil {
		t.Fatal(err)
	}

	if f.OSID != "ubuntu" || f.OSVersion != "24.04" || f.Codename != "noble" {
		t.Errorf("unexpected OS: %q %q %q", f.OSID, f.OSVersion, f.Codename)
	}
	if f.OSEOL.Format(time.DateOnly) != "2029-05-31" {
		t.Errorf("unexpected EOL: %v", f.OSEOL)
	}
	if f.UpdatesPending != 333 || f.SecurityPending != 209 || f.SecurityApprox {
		t.Errorf("unexpected updates: %d;%d approx=%v", f.UpdatesPending, f.SecurityPending, f.SecurityApprox)
	}
	if !f.UUInstalled || f.UUSetting != "1" || f.UUTimer != "enabled" || f.UURan.IsZero() {
		t.Errorf("unexpected unattended-upgrades facts: %+v", f)
	}
	if f.PHPFPM["8.3"] != "inactive" || f.PHPFPM["8.4"] != "active" {
		t.Errorf("unexpected PHP-FPM: %v", f.PHPFPM)
	}
	if got := strings.Join(f.PHPBranches(), ","); got != "8.3,8.4" {
		t.Errorf("PHPBranches() = %q", got)
	}

	php := f.Packages["php8.4-fpm"]
	if php == nil || php.Installed != "8.4.18-2+ubuntu24.04.1+deb.sury.org+1" || php.Cmp != "lt" {
		t.Errorf("unexpected php8.4-fpm: %+v", php)
	}
	if php.Origin != "https://ppa.launchpadcontent.net/ondrej/php/ubuntu noble/main" {
		t.Errorf("unexpected php8.4-fpm origin: %q", php.Origin)
	}
	if m := f.Packages["mariadb-server"]; m == nil || m.Installed != "1:10.11.14+maria~ubu2404" {
		t.Errorf("unexpected mariadb-server: %+v", m)
	}
}

func TestParseProbeIgnoresNoiseAndCRLF(t *testing.T) {
	out := "Welcome to Ubuntu\r\nsome motd=thing\r\n" +
		"doctor_begin v=1\r\n" +
		"now=1795482057\r\n" +
		"pkg.php8.3-fpm.installed=(none)\r\n" +
		"pkg.php8.3-fpm.candidate=8.3.37-1\r\n" +
		"not a key value line\r\n" +
		"doctor_end\r\n" +
		"after=ignored\r\n"

	f, err := ParseProbe(out)
	if err != nil {
		t.Fatal(err)
	}

	if f.Now.Unix() != 1795482057 {
		t.Errorf("unexpected now: %v", f.Now)
	}
	p := f.Packages["php8.3-fpm"]
	if p == nil || p.Installed != "" || p.Candidate != "8.3.37-1" {
		t.Errorf("unexpected package: %+v", p)
	}
	if _, ok := f.Packages["after"]; ok {
		t.Error("parsed a line after the end marker")
	}
	if f.SecurityPending != -1 || f.UpdatesPending != -1 {
		t.Errorf("missing apt-check should be unknown, got %d;%d", f.UpdatesPending, f.SecurityPending)
	}
}

func TestParseProbeAptError(t *testing.T) {
	f, err := ParseProbe("doctor_begin v=1\npkg.nginx.error=1\npkg.nginx.installed=1.29.1-1~noble\ndoctor_end\n")
	if err != nil {
		t.Fatal(err)
	}
	if p := f.Packages["nginx"]; p == nil || !p.AptError || p.Installed != "1.29.1-1~noble" {
		t.Errorf("unexpected package: %+v", p)
	}
}

func TestParseProbeApproxSecurityCount(t *testing.T) {
	f, err := ParseProbe("doctor_begin v=1\napt.upgradable_security=4\ndoctor_end\n")
	if err != nil {
		t.Fatal(err)
	}
	if f.SecurityPending != 4 || !f.SecurityApprox {
		t.Errorf("got %d approx=%v", f.SecurityPending, f.SecurityApprox)
	}
}

func TestParseProbeErrors(t *testing.T) {
	if _, err := ParseProbe("doctor_begin v=1\nnow=1\n"); !errors.Is(err, ErrProbeTruncated) {
		t.Errorf("expected truncated error, got %v", err)
	}
	if _, err := ParseProbe("bash: line 1: sh: not found\n"); err == nil {
		t.Error("expected error for missing output")
	}
	if _, err := ParseProbe("doctor_begin v=2\ndoctor_end\n"); err == nil {
		t.Error("expected error for unsupported version")
	}
}
