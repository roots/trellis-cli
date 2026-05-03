package trust

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRebuildBundleConcatenatesAllUserCAs(t *testing.T) {
	withDataDir(t)
	if err := os.MkdirAll(userCADir(), 0o755); err != nil {
		t.Fatal(err)
	}

	a, _ := generateTestCertPEM(t, "a.test")
	b, _ := generateTestCertPEM(t, "b.test")
	if err := os.WriteFile(filepath.Join(userCADir(), "a.crt"), a, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userCADir(), "b.crt"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rebuildBundle(); err != nil {
		t.Fatalf("rebuildBundle: %v", err)
	}

	fpA, _ := Fingerprint(a)
	fpB, _ := Fingerprint(b)
	if !pemFileContainsFingerprint(userCABundle(), fpA) {
		t.Error("bundle missing cert A")
	}
	if !pemFileContainsFingerprint(userCABundle(), fpB) {
		t.Error("bundle missing cert B")
	}
}

func TestRebuildBundleInsertsTrailingNewlineBetweenEntries(t *testing.T) {
	withDataDir(t)
	if err := os.MkdirAll(userCADir(), 0o755); err != nil {
		t.Fatal(err)
	}

	a, _ := generateTestCertPEM(t, "a.test")
	b, _ := generateTestCertPEM(t, "b.test")
	// Strip trailing newlines from cert A so rebuildBundle has to inject one.
	aNoNewline := []byte(strings.TrimRight(string(a), "\n"))
	if err := os.WriteFile(filepath.Join(userCADir(), "a.crt"), aNoNewline, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userCADir(), "b.crt"), b, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rebuildBundle(); err != nil {
		t.Fatalf("rebuildBundle: %v", err)
	}

	// Both certs must be parseable from the resulting bundle. If the
	// newline weren't injected, cert B's BEGIN block would run on from
	// cert A's END line and pem.Decode would fail to decode B.
	fpB, _ := Fingerprint(b)
	if !pemFileContainsFingerprint(userCABundle(), fpB) {
		t.Error("bundle does not contain cert B — trailing-newline injection broken")
	}
}

func TestRebuildBundleRemovesBundleWhenDirEmpty(t *testing.T) {
	withDataDir(t)
	if err := os.MkdirAll(userCADir(), 0o755); err != nil {
		t.Fatal(err)
	}

	// Pre-create a bundle to ensure we'd notice if it weren't deleted.
	if err := os.WriteFile(userCABundle(), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := rebuildBundle(); err != nil {
		t.Fatalf("rebuildBundle: %v", err)
	}
	if _, err := os.Stat(userCABundle()); !os.IsNotExist(err) {
		t.Errorf("bundle still exists after empty rebuild; err=%v", err)
	}
}

func TestPemFileContainsFingerprintScansAllBlocks(t *testing.T) {
	tmp := t.TempDir()
	a, _ := generateTestCertPEM(t, "first.test")
	b, _ := generateTestCertPEM(t, "second.test")

	combined := append([]byte(nil), a...)
	combined = append(combined, b...)
	path := filepath.Join(tmp, "combined.pem")
	if err := os.WriteFile(path, combined, 0o644); err != nil {
		t.Fatal(err)
	}

	fpA, _ := Fingerprint(a)
	fpB, _ := Fingerprint(b)

	if !pemFileContainsFingerprint(path, fpA) {
		t.Error("first cert not found")
	}
	if !pemFileContainsFingerprint(path, fpB) {
		t.Error("second cert not found")
	}
	if pemFileContainsFingerprint(path, "deadbeef") {
		t.Error("returned true for unknown fingerprint")
	}
}

func TestPemFileContainsFingerprintMissingFileReturnsFalse(t *testing.T) {
	if pemFileContainsFingerprint("/no/such/path", "deadbeef") {
		t.Error("returned true for missing file")
	}
}

func TestFileFingerprintMatches(t *testing.T) {
	tmp := t.TempDir()
	pem, _ := generateTestCertPEM(t, "x.test")
	path := filepath.Join(tmp, "cert.pem")
	if err := os.WriteFile(path, pem, 0o644); err != nil {
		t.Fatal(err)
	}

	fp, _ := Fingerprint(pem)
	if !fileFingerprintMatches(path, fp) {
		t.Error("fileFingerprintMatches = false for matching cert")
	}
	if fileFingerprintMatches(path, "deadbeef") {
		t.Error("fileFingerprintMatches = true for mismatched fingerprint")
	}
	if fileFingerprintMatches("/nope", fp) {
		t.Error("fileFingerprintMatches = true for missing file")
	}
}

// newTestLinuxStore wires a linuxStore around a fake runner with NSS
// disabled so tests focus on the Linux CA bundle code path.
func newTestLinuxStore(r *fakeRunner, opts Options) *linuxStore {
	opts.DisableNSS = true
	return &linuxStore{
		opts:   opts,
		runner: r,
		nss:    nssHelper{runner: r},
	}
}

func TestLinuxStoreTrustWritesUserCAAndBundle(t *testing.T) {
	withDataDir(t)
	pem, _ := generateTestCertPEM(t, "example.test")
	fp, _ := Fingerprint(pem)

	r := &fakeRunner{}
	s := newTestLinuxStore(r, Options{})

	res, err := s.Trust(TrustInput{
		CertPath:    "/tmp/cert.pem",
		CertPEM:     pem,
		Fingerprint: fp,
		Label:       "trellis-deadbeef-example.test",
	})
	if err != nil {
		t.Fatalf("Trust err = %v", err)
	}
	if len(res.Locations) != 1 || res.Locations[0] != linuxUserCALocation {
		t.Errorf("Locations = %v, want [user-ca]", res.Locations)
	}
	if !pemFileContainsFingerprint(userCABundle(), fp) {
		t.Errorf("bundle does not contain trusted cert fingerprint")
	}
	if r.callCount(func(c fakeCall) bool { return c.Name == "sudo" }) != 0 {
		t.Errorf("sudo was invoked even though TrustSystem=false; calls=%v", r.calls)
	}
}

func TestLinuxStoreTrustSystemRunsSudoTeeAndUpdate(t *testing.T) {
	withDataDir(t)
	pem, _ := generateTestCertPEM(t, "example.test")
	fp, _ := Fingerprint(pem)

	r := &fakeRunner{}
	r.on("sudo", "tee").succeed()
	r.on("sudo", "update-ca-certificates").succeed()

	s := newTestLinuxStore(r, Options{TrustSystem: true})

	res, err := s.Trust(TrustInput{
		CertPath:    "/tmp/cert.pem",
		CertPEM:     pem,
		Fingerprint: fp,
		Label:       "trellis-deadbeef-example.test",
	})
	if err != nil {
		t.Fatalf("Trust err = %v", err)
	}
	if len(res.Locations) != 2 {
		t.Errorf("Locations = %v, want both user + system", res.Locations)
	}
	if !r.hasCall("sudo", "tee") {
		t.Errorf("sudo tee not invoked; calls=%v", r.calls)
	}
	if !r.hasCall("sudo", "update-ca-certificates") {
		t.Errorf("sudo update-ca-certificates not invoked; calls=%v", r.calls)
	}
}

func TestLinuxStoreTrustSystemRecordsLocationEvenWhenUpdateFails(t *testing.T) {
	withDataDir(t)
	pem, _ := generateTestCertPEM(t, "example.test")
	fp, _ := Fingerprint(pem)

	r := &fakeRunner{}
	r.on("sudo", "tee").succeed()
	r.on("sudo", "update-ca-certificates").failWith("hash collision")

	s := newTestLinuxStore(r, Options{TrustSystem: true})

	res, err := s.Trust(TrustInput{
		CertPath:    "/tmp/cert.pem",
		CertPEM:     pem,
		Fingerprint: fp,
		Label:       "trellis-deadbeef-example.test",
	})
	if err == nil {
		t.Fatal("Trust err = nil, want error from update-ca-certificates")
	}
	hasSystem := false
	for _, l := range res.Locations {
		if l == linuxSystemCALocation {
			hasSystem = true
		}
	}
	if !hasSystem {
		t.Errorf("Locations = %v, want system-CA recorded so untrust can clean up", res.Locations)
	}
}

func TestLinuxStoreUntrustUserCARemovesFile(t *testing.T) {
	withDataDir(t)
	if err := os.MkdirAll(userCADir(), 0o755); err != nil {
		t.Fatal(err)
	}
	label := "trellis-deadbeef-example.test"
	pem, _ := generateTestCertPEM(t, "example.test")
	caPath := filepath.Join(userCADir(), safeFilename(label)+".crt")
	if err := os.WriteFile(caPath, pem, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &fakeRunner{}
	s := newTestLinuxStore(r, Options{})

	cleaned, err := s.Untrust(TrustInput{Label: label}, []string{linuxUserCALocation})
	if err != nil {
		t.Fatalf("Untrust err = %v", err)
	}
	if len(cleaned) != 1 {
		t.Errorf("cleaned = %v, want one location", cleaned)
	}
	if _, err := os.Stat(caPath); !os.IsNotExist(err) {
		t.Errorf("user CA file still present after untrust; err=%v", err)
	}
}

func TestLinuxStoreUntrustSystemRunsSudoRmAndFresh(t *testing.T) {
	withDataDir(t)
	r := &fakeRunner{}
	r.on("sudo", "rm").succeed()
	r.on("sudo", "update-ca-certificates").succeed()

	s := newTestLinuxStore(r, Options{})
	cleaned, err := s.Untrust(TrustInput{Label: "trellis-deadbeef-example.test"}, []string{linuxSystemCALocation})
	if err != nil {
		t.Fatalf("Untrust err = %v", err)
	}
	if len(cleaned) != 1 {
		t.Errorf("cleaned = %v, want one location", cleaned)
	}
	if !r.hasCall("sudo", "update-ca-certificates", "--fresh") {
		t.Errorf("untrust did not invoke sudo update-ca-certificates --fresh; calls=%v", r.calls)
	}
}

func TestLinuxStoreVerifyUserCAPresentRequiresBothFileAndBundle(t *testing.T) {
	withDataDir(t)
	if err := os.MkdirAll(userCADir(), 0o755); err != nil {
		t.Fatal(err)
	}
	pem, _ := generateTestCertPEM(t, "example.test")
	fp, _ := Fingerprint(pem)
	label := "trellis-deadbeef-example.test"
	caPath := filepath.Join(userCADir(), safeFilename(label)+".crt")
	if err := os.WriteFile(caPath, pem, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &fakeRunner{}
	s := newTestLinuxStore(r, Options{})

	// File present but bundle missing → Missing.
	res, err := s.Verify(TrustInput{Label: label, Fingerprint: fp}, []string{linuxUserCALocation})
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	if len(res.Missing) != 1 {
		t.Errorf("Missing = %v, want one (bundle missing)", res.Missing)
	}

	// Now build the bundle and re-verify → Present.
	if err := rebuildBundle(); err != nil {
		t.Fatal(err)
	}
	res, err = s.Verify(TrustInput{Label: label, Fingerprint: fp}, []string{linuxUserCALocation})
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	if len(res.Present) != 1 {
		t.Errorf("Present = %v, want one after bundle rebuild", res.Present)
	}
}

func TestLinuxStoreVerifyUserCAFingerprintMismatchIsMissing(t *testing.T) {
	withDataDir(t)
	if err := os.MkdirAll(userCADir(), 0o755); err != nil {
		t.Fatal(err)
	}
	pem, _ := generateTestCertPEM(t, "example.test")
	label := "trellis-deadbeef-example.test"
	caPath := filepath.Join(userCADir(), safeFilename(label)+".crt")
	if err := os.WriteFile(caPath, pem, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rebuildBundle(); err != nil {
		t.Fatal(err)
	}

	r := &fakeRunner{}
	s := newTestLinuxStore(r, Options{})

	res, err := s.Verify(TrustInput{Label: label, Fingerprint: "deadbeef"}, []string{linuxUserCALocation})
	if err != nil {
		t.Fatalf("Verify err = %v", err)
	}
	if len(res.Missing) != 1 {
		t.Errorf("Missing = %v, want one for fingerprint mismatch", res.Missing)
	}
}

func TestSafeFilename(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"alphanumeric", "trellis-abc12345-example.com", "trellis-abc12345-example.com"},
		{"keeps dot dash underscore", "a.b-c_d", "a.b-c_d"},
		{"spaces become underscore", "hello world", "hello_world"},
		{"slashes neutralized", "../../etc/passwd", ".._.._etc_passwd"},
		{"control chars neutralized", "a\nb\tc", "a_b_c"},
		{"unicode is dropped to underscores", "exämple", "ex__mple"},
		{"empty stays empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeFilename(tc.in); got != tc.want {
				t.Errorf("safeFilename(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
