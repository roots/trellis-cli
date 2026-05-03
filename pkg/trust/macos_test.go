package trust

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestMacOSStore wires a macOSStore around a fake runner with NSS
// disabled so tests focus on the keychain code path.
func newTestMacOSStore(r *fakeRunner) *macOSStore {
	return &macOSStore{
		opts:   Options{DisableNSS: true},
		runner: r,
		nss:    nssHelper{runner: r},
	}
}

func TestIsKeychainNotFound(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"could not be found", "Cert could not be found", true},
		{"specified item could not be found", "SecKeychainSearchCopyNext: The specified item could not be found in the keychain.", true},
		{"no matching items", "SecKeychainSearchCopyNext: No matching items found", true},
		{"case insensitive", "COULD NOT BE FOUND", true},
		{"unrelated error", "User canceled the operation.", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isKeychainNotFound(tc.in); got != tc.want {
				t.Errorf("isKeychainNotFound(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSha1HexFromCertFile(t *testing.T) {
	tmp := t.TempDir()
	pem, der := generateTestCertPEM(t, "x.test")
	path := filepath.Join(tmp, "cert.pem")
	if err := os.WriteFile(path, pem, 0o644); err != nil {
		t.Fatal(err)
	}

	expected := sha1.Sum(der)
	want := strings.ToUpper(hex.EncodeToString(expected[:]))

	got := sha1HexFromCertFile(path)
	if got != want {
		t.Errorf("sha1HexFromCertFile = %q, want %q", got, want)
	}
}

func TestMacOSStoreTrustCallsAddTrustedCert(t *testing.T) {
	r := &fakeRunner{}
	r.on("security", "add-trusted-cert").succeed()

	s := newTestMacOSStore(r)
	res, err := s.Trust(TrustInput{CertPath: "/tmp/cert.pem", Label: "trellis-x-example"})
	if err != nil {
		t.Fatalf("Trust err = %v", err)
	}
	if len(res.Locations) != 1 || res.Locations[0] != macOSLoginKeychainLocation {
		t.Errorf("Locations = %v, want [%q]", res.Locations, macOSLoginKeychainLocation)
	}
	if !r.hasCall("security", "add-trusted-cert", "-r", "trustRoot", "-p", "ssl") {
		t.Errorf("add-trusted-cert not called with expected -p ssl / -r trustRoot flags; calls=%v", r.calls)
	}
}

func TestMacOSStoreTrustReturnsErrorWhenSecurityFails(t *testing.T) {
	r := &fakeRunner{}
	r.on("security", "add-trusted-cert").failWith("permission denied")

	s := newTestMacOSStore(r)
	if _, err := s.Trust(TrustInput{CertPath: "/tmp/cert.pem"}); err == nil {
		t.Fatal("Trust err = nil, want error")
	}
}

func TestMacOSStoreUntrustDeletesWhenCertPresent(t *testing.T) {
	tmp := t.TempDir()
	pem, der := generateTestCertPEM(t, "example.test")
	certPath := filepath.Join(tmp, "cert.pem")
	if err := os.WriteFile(certPath, pem, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum(der)
	wantSHA1 := strings.ToUpper(hex.EncodeToString(sum[:]))

	r := &fakeRunner{}
	r.on("security", "remove-trusted-cert").succeed()
	r.on("security", "find-certificate").succeedWith("SHA-1 hash: " + wantSHA1 + "\n")
	r.on("security", "delete-certificate").succeed()

	s := newTestMacOSStore(r)
	cleaned, err := s.Untrust(TrustInput{CertPath: certPath}, []string{macOSLoginKeychainLocation})
	if err != nil {
		t.Fatalf("Untrust err = %v", err)
	}
	if len(cleaned) != 1 {
		t.Errorf("cleaned = %v, want [keychain]", cleaned)
	}
	if !r.hasCall("security", "delete-certificate", "-Z", wantSHA1) {
		t.Errorf("delete-certificate not called with expected SHA-1; calls=%v", r.calls)
	}
}

func TestMacOSStoreUntrustSkipsDeleteWhenAlreadyGone(t *testing.T) {
	tmp := t.TempDir()
	pem, _ := generateTestCertPEM(t, "example.test")
	certPath := filepath.Join(tmp, "cert.pem")
	if err := os.WriteFile(certPath, pem, 0o644); err != nil {
		t.Fatal(err)
	}

	r := &fakeRunner{}
	r.on("security", "remove-trusted-cert").succeed()
	// find-certificate returns no SHA-1 line for our cert.
	r.on("security", "find-certificate").succeedWith("SHA-1 hash: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n")

	s := newTestMacOSStore(r)
	cleaned, err := s.Untrust(TrustInput{CertPath: certPath}, []string{macOSLoginKeychainLocation})
	if err != nil {
		t.Fatalf("Untrust err = %v", err)
	}
	if len(cleaned) != 1 {
		t.Errorf("cleaned = %v, want one location", cleaned)
	}
	if r.hasCall("security", "delete-certificate") {
		t.Errorf("delete-certificate called even though cert was missing; calls=%v", r.calls)
	}
}

func TestMacOSStoreUntrustTreatsNotFoundAsSuccess(t *testing.T) {
	tmp := t.TempDir()
	pem, der := generateTestCertPEM(t, "example.test")
	certPath := filepath.Join(tmp, "cert.pem")
	if err := os.WriteFile(certPath, pem, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum(der)
	wantSHA1 := strings.ToUpper(hex.EncodeToString(sum[:]))

	r := &fakeRunner{}
	r.on("security", "remove-trusted-cert").succeed()
	r.on("security", "find-certificate").succeedWith("SHA-1 hash: " + wantSHA1 + "\n")
	r.on("security", "delete-certificate").failWith("Could not be found")

	s := newTestMacOSStore(r)
	cleaned, err := s.Untrust(TrustInput{CertPath: certPath}, []string{macOSLoginKeychainLocation})
	if err != nil {
		t.Fatalf("Untrust err = %v, want nil (not-found is success)", err)
	}
	if len(cleaned) != 1 {
		t.Errorf("cleaned = %v, want one location", cleaned)
	}
}

func TestMacOSStoreUntrustReturnsOtherErrors(t *testing.T) {
	tmp := t.TempDir()
	pem, der := generateTestCertPEM(t, "example.test")
	certPath := filepath.Join(tmp, "cert.pem")
	if err := os.WriteFile(certPath, pem, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha1.Sum(der)
	wantSHA1 := strings.ToUpper(hex.EncodeToString(sum[:]))

	r := &fakeRunner{}
	r.on("security", "remove-trusted-cert").succeed()
	r.on("security", "find-certificate").succeedWith("SHA-1 hash: " + wantSHA1 + "\n")
	r.on("security", "delete-certificate").failWith("user interaction required")

	s := newTestMacOSStore(r)
	if _, err := s.Untrust(TrustInput{CertPath: certPath}, []string{macOSLoginKeychainLocation}); err == nil {
		t.Fatal("Untrust err = nil, want error")
	}
}

func TestMacOSStoreVerifyClassifiesByCertCheckOutcome(t *testing.T) {
	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "cert.pem")
	if err := os.WriteFile(certPath, []byte("anything"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		setup func(r *fakeRunner)
		want  func(v VerifyResult) bool
	}{
		{
			"present when verify-cert succeeds",
			func(r *fakeRunner) { r.on("security", "verify-cert").succeed() },
			func(v VerifyResult) bool { return len(v.Present) == 1 && len(v.Missing) == 0 },
		},
		{
			"missing when verify-cert says not trusted",
			func(r *fakeRunner) { r.on("security", "verify-cert").failWith("CSSMERR_TP_NOT_TRUSTED") },
			func(v VerifyResult) bool { return len(v.Missing) == 1 && len(v.Present) == 0 },
		},
		{
			"unknown when verify-cert errors with unrelated message",
			func(r *fakeRunner) { r.on("security", "verify-cert").failWith("unable to read keychain") },
			func(v VerifyResult) bool { return len(v.Unknown) == 1 && len(v.Present)+len(v.Missing) == 0 },
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRunner{}
			tc.setup(r)
			s := newTestMacOSStore(r)
			res, err := s.Verify(TrustInput{CertPath: certPath}, []string{macOSLoginKeychainLocation})
			if err != nil {
				t.Fatalf("Verify err = %v", err)
			}
			if !tc.want(res) {
				t.Errorf("VerifyResult = %+v, want classification %s", res, tc.name)
			}
		})
	}
}

func TestSha1HexFromCertFileEmptyOnError(t *testing.T) {
	if got := sha1HexFromCertFile(""); got != "" {
		t.Errorf("empty path: got %q, want \"\"", got)
	}
	if got := sha1HexFromCertFile("/no/such/path"); got != "" {
		t.Errorf("missing file: got %q, want \"\"", got)
	}

	tmp := t.TempDir()
	junkPath := filepath.Join(tmp, "junk")
	if err := os.WriteFile(junkPath, []byte("not a pem cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := sha1HexFromCertFile(junkPath); got != "" {
		t.Errorf("non-PEM content: got %q, want \"\"", got)
	}
}
