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
