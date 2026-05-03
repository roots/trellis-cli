package trust

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestIsNSSNicknameNotFound(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"could not find", "PR_FILE_NOT_FOUND_ERROR: could not find certificate", true},
		{"no such certificate", "no such certificate named foo", true},
		{"unrecognized oid", "SEC_ERROR_UNRECOGNIZED_OID", true},
		{"case insensitive", "Could Not Find named foo", true},
		{"unrelated error", "permission denied opening db", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isNSSNicknameNotFound(tc.in); got != tc.want {
				t.Errorf("isNSSNicknameNotFound(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsNSSProfile(t *testing.T) {
	tmp := t.TempDir()

	notProfile := filepath.Join(tmp, "no-marker")
	if err := os.MkdirAll(notProfile, 0o755); err != nil {
		t.Fatal(err)
	}
	if isNSSProfile(notProfile) {
		t.Error("isNSSProfile = true for dir without cert9.db")
	}

	cert9 := filepath.Join(tmp, "with-cert9")
	if err := os.MkdirAll(cert9, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cert9, "cert9.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isNSSProfile(cert9) {
		t.Error("isNSSProfile = false for dir with cert9.db")
	}

	cert8 := filepath.Join(tmp, "with-cert8")
	if err := os.MkdirAll(cert8, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cert8, "cert8.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isNSSProfile(cert8) {
		t.Error("isNSSProfile = false for dir with cert8.db (legacy)")
	}
}

func TestFirefoxProfileDirsFindsValidProfilesUnderHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	var profilesParent string
	switch runtime.GOOS {
	case "darwin":
		profilesParent = filepath.Join(tmp, "Library", "Application Support", "Firefox", "Profiles")
	default:
		profilesParent = filepath.Join(tmp, ".mozilla", "firefox")
	}

	good := filepath.Join(profilesParent, "abc.default")
	bad := filepath.Join(profilesParent, "no-marker.default")
	for _, d := range []string{good, bad} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(good, "cert9.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also drop a stray file at the parent level — should be ignored, not crash.
	if err := os.WriteFile(filepath.Join(profilesParent, "profiles.ini"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := firefoxProfileDirs()
	sort.Strings(got)
	if len(got) != 1 || got[0] != good {
		t.Errorf("firefoxProfileDirs = %v, want [%q]", got, good)
	}
}

func TestFirefoxProfileDirsEmptyWhenNoFirefoxInstalled(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)

	got := firefoxProfileDirs()
	if len(got) != 0 {
		t.Errorf("firefoxProfileDirs = %v, want empty when no Firefox dir exists", got)
	}
}

// setupOneFirefoxProfile creates a single fake Firefox profile under HOME
// for the test's lifetime and returns its absolute path.
func setupOneFirefoxProfile(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	var parent string
	switch runtime.GOOS {
	case "darwin":
		parent = filepath.Join(tmp, "Library", "Application Support", "Firefox", "Profiles")
	default:
		parent = filepath.Join(tmp, ".mozilla", "firefox")
	}
	profile := filepath.Join(parent, "abc.default")
	if err := os.MkdirAll(profile, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profile, "cert9.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return profile
}

func TestNSSTrustNoOpWhenDisabled(t *testing.T) {
	r := &fakeRunner{}
	h := nssHelper{runner: r}
	locs, status, err := h.trust(TrustInput{}, true)
	if err != nil || len(locs) != 0 || status.FirefoxFound || status.CertutilMissing {
		t.Errorf("disabled call should be a no-op; locs=%v status=%+v err=%v", locs, status, err)
	}
	if len(r.calls) != 0 {
		t.Errorf("disabled call invoked the runner: %v", r.calls)
	}
}

func TestNSSTrustReportsCertutilMissing(t *testing.T) {
	setupOneFirefoxProfile(t)
	r := &fakeRunner{}
	r.markMissing("certutil")

	h := nssHelper{runner: r}
	locs, status, err := h.trust(TrustInput{Label: "trellis-x"}, false)
	if err != nil {
		t.Fatalf("trust err = %v", err)
	}
	if len(locs) != 0 {
		t.Errorf("locs = %v, want empty when certutil missing", locs)
	}
	if !status.FirefoxFound {
		t.Errorf("FirefoxFound = false, want true (a profile exists)")
	}
	if !status.CertutilMissing {
		t.Errorf("CertutilMissing = false, want true")
	}
}

func TestNSSTrustReportsFirefoxNotFoundButCertutilPresent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	r := &fakeRunner{}

	h := nssHelper{runner: r}
	locs, status, err := h.trust(TrustInput{Label: "trellis-x"}, false)
	if err != nil {
		t.Fatalf("trust err = %v", err)
	}
	if status.FirefoxFound {
		t.Errorf("FirefoxFound = true, want false")
	}
	if status.CertutilMissing {
		t.Errorf("CertutilMissing = true, want false (certutil is present)")
	}
	if len(locs) != 0 {
		t.Errorf("locs = %v, want empty", locs)
	}
}

func TestNSSTrustHappyPathDeletesThenAdds(t *testing.T) {
	profile := setupOneFirefoxProfile(t)

	r := &fakeRunner{}
	r.on("/usr/bin/certutil", "-D").succeed()
	r.on("/usr/bin/certutil", "-A").succeed()

	h := nssHelper{runner: r}
	locs, status, err := h.trust(TrustInput{Label: "trellis-x", CertPath: "/tmp/x.cert"}, false)
	if err != nil {
		t.Fatalf("trust err = %v", err)
	}
	if !status.FirefoxFound || status.CertutilMissing {
		t.Errorf("status = %+v, want firefox found + certutil present", status)
	}
	if len(locs) != 1 || locs[0] != "nss:"+profile {
		t.Errorf("locs = %v, want one nss:profile entry", locs)
	}
	// Both -D (drop existing) and -A (add fresh) must run, in order.
	if len(r.calls) != 2 {
		t.Fatalf("calls = %d, want 2 (one -D, one -A)", len(r.calls))
	}
	if r.calls[0].Args[0] != "-D" || r.calls[1].Args[0] != "-A" {
		t.Errorf("call order wrong; got %s then %s, want -D then -A", r.calls[0].Args[0], r.calls[1].Args[0])
	}
}

func TestNSSTrustReturnsErrorOnAddFailure(t *testing.T) {
	setupOneFirefoxProfile(t)

	r := &fakeRunner{}
	r.on("/usr/bin/certutil", "-D").succeed()
	r.on("/usr/bin/certutil", "-A").failWith("PR_FILE_DISK_FULL_ERROR")

	h := nssHelper{runner: r}
	locs, _, err := h.trust(TrustInput{Label: "trellis-x", CertPath: "/tmp/x.cert"}, false)
	if err == nil {
		t.Fatal("trust err = nil, want error from -A failure")
	}
	if len(locs) != 0 {
		t.Errorf("locs = %v, want empty when -A failed", locs)
	}
}

func TestNSSUntrustHappyPath(t *testing.T) {
	r := &fakeRunner{}
	r.on("/usr/bin/certutil", "-D").succeed()

	h := nssHelper{runner: r}
	cleaned, err := h.untrust(TrustInput{Label: "trellis-x"}, []string{"nss:/profiles/abc"})
	if err != nil {
		t.Fatalf("untrust err = %v", err)
	}
	if len(cleaned) != 1 {
		t.Errorf("cleaned = %v, want one location", cleaned)
	}
}

func TestNSSUntrustNicknameNotFoundIsSuccess(t *testing.T) {
	r := &fakeRunner{}
	r.on("/usr/bin/certutil", "-D").failWith("could not find certificate named foo")

	h := nssHelper{runner: r}
	cleaned, err := h.untrust(TrustInput{Label: "trellis-x"}, []string{"nss:/profiles/abc"})
	if err != nil {
		t.Fatalf("untrust err = %v, want nil (nickname-not-found is success)", err)
	}
	if len(cleaned) != 1 {
		t.Errorf("cleaned = %v, want one location", cleaned)
	}
}

func TestNSSUntrustReturnsErrorWhenCertutilMissingButRecorded(t *testing.T) {
	r := &fakeRunner{}
	r.markMissing("certutil")

	h := nssHelper{runner: r}
	if _, err := h.untrust(TrustInput{Label: "trellis-x"}, []string{"nss:/profiles/abc"}); err == nil {
		t.Fatal("untrust err = nil, want error (certutil missing but state has NSS records)")
	}
}

func TestNSSUntrustNoOpWhenCertutilMissingAndNoRecord(t *testing.T) {
	r := &fakeRunner{}
	r.markMissing("certutil")

	h := nssHelper{runner: r}
	cleaned, err := h.untrust(TrustInput{Label: "trellis-x"}, []string{macOSLoginKeychainLocation})
	if err != nil {
		t.Errorf("untrust err = %v, want nil (no NSS records to clean)", err)
	}
	if len(cleaned) != 0 {
		t.Errorf("cleaned = %v, want empty", cleaned)
	}
}

func TestNSSVerifyMatchingFingerprintIsPresent(t *testing.T) {
	pem, _ := generateTestCertPEM(t, "x.test")
	fp, _ := Fingerprint(pem)

	r := &fakeRunner{}
	r.on("/usr/bin/certutil", "-L").returns(fakeResponse{Stdout: pem})

	h := nssHelper{runner: r}
	present, missing, unknown := h.verify(TrustInput{Label: "trellis-x", Fingerprint: fp}, []string{"nss:/profiles/abc"})
	if len(present) != 1 || len(missing) != 0 || len(unknown) != 0 {
		t.Errorf("present=%v missing=%v unknown=%v, want one Present", present, missing, unknown)
	}
}

func TestNSSVerifyDifferentFingerprintIsMissing(t *testing.T) {
	pem, _ := generateTestCertPEM(t, "x.test")

	r := &fakeRunner{}
	r.on("/usr/bin/certutil", "-L").returns(fakeResponse{Stdout: pem})

	h := nssHelper{runner: r}
	_, missing, _ := h.verify(TrustInput{Label: "trellis-x", Fingerprint: "deadbeef"}, []string{"nss:/profiles/abc"})
	if len(missing) != 1 {
		t.Errorf("missing = %v, want one for fingerprint mismatch", missing)
	}
}

func TestNSSVerifyCertutilErrorIsMissing(t *testing.T) {
	r := &fakeRunner{}
	r.on("/usr/bin/certutil", "-L").failWith("PR_FILE_NOT_FOUND_ERROR")

	h := nssHelper{runner: r}
	_, missing, _ := h.verify(TrustInput{Label: "trellis-x", Fingerprint: "abc"}, []string{"nss:/profiles/abc"})
	if len(missing) != 1 {
		t.Errorf("missing = %v, want one when certutil errors", missing)
	}
}

func TestNSSVerifyUnknownWhenCertutilNotOnPath(t *testing.T) {
	r := &fakeRunner{}
	r.markMissing("certutil")

	h := nssHelper{runner: r}
	_, missing, unknown := h.verify(TrustInput{Label: "trellis-x", Fingerprint: "abc"}, []string{"nss:/profiles/abc", macOSLoginKeychainLocation})
	if len(missing) != 0 {
		t.Errorf("missing = %v, want zero (certutil missing → Unknown, not Missing)", missing)
	}
	if len(unknown) != 1 {
		t.Errorf("unknown = %v, want exactly one (the NSS location only)", unknown)
	}
}
