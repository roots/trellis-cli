package trust

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeStore is a recording Store implementation for ApplySite/RevokeSite
// tests. Each test sets the canned responses on the fields and inspects
// the recorded calls afterwards.
type fakeStore struct {
	trustResult TrustResult
	trustErr    error

	verifyResult VerifyResult
	verifyErr    error

	untrustCleaned []string
	untrustErr     error

	trustCalls   []TrustInput
	untrustCalls []untrustCall
	verifyCalls  []verifyCall
}

type untrustCall struct {
	input     TrustInput
	locations []string
}

type verifyCall struct {
	input     TrustInput
	locations []string
}

func (f *fakeStore) Trust(input TrustInput) (TrustResult, error) {
	f.trustCalls = append(f.trustCalls, input)
	return f.trustResult, f.trustErr
}

func (f *fakeStore) Untrust(input TrustInput, locations []string) ([]string, error) {
	f.untrustCalls = append(f.untrustCalls, untrustCall{input: input, locations: append([]string(nil), locations...)})
	return f.untrustCleaned, f.untrustErr
}

func (f *fakeStore) Verify(input TrustInput, locations []string) (VerifyResult, error) {
	f.verifyCalls = append(f.verifyCalls, verifyCall{input: input, locations: append([]string(nil), locations...)})
	return f.verifyResult, f.verifyErr
}

// applyEnv builds a SiteInput pointing at a fresh temp dir, with a generated
// cert PEM (and optional key bytes). Returns the input and the on-disk
// cert/key paths ApplySite will use.
func applyEnv(t *testing.T, project, site string, withKey bool) (SiteInput, string, string) {
	t.Helper()
	tmp := t.TempDir()
	certPEM, _ := generateTestCertPEM(t, site)
	in := SiteInput{
		Project:      project,
		Site:         site,
		InstanceName: "example.com",
		BaseDir:      tmp,
		CertPEM:      certPEM,
	}
	if withKey {
		in.KeyPEM = []byte("-----BEGIN PRIVATE KEY-----\nfake\n-----END PRIVATE KEY-----\n")
	}
	exportDir := ExportDir(tmp, in.InstanceName, project)
	return in, filepath.Join(exportDir, site+".cert"), filepath.Join(exportDir, site+".key")
}

func TestApplySiteFreshTrust(t *testing.T) {
	in, certPath, keyPath := applyEnv(t, "/p1", "example.test", true)

	store := &fakeStore{
		trustResult: TrustResult{Locations: []string{macOSLoginKeychainLocation}},
	}
	state := &State{}

	out := ApplySite(store, state, in)

	if out.Err != nil {
		t.Fatalf("Err = %v, want nil", out.Err)
	}
	if out.Verb != "trusted" {
		t.Errorf("Verb = %q, want %q", out.Verb, "trusted")
	}
	if len(store.trustCalls) != 1 {
		t.Fatalf("Trust called %d times, want 1", len(store.trustCalls))
	}
	if len(store.untrustCalls) != 0 {
		t.Errorf("Untrust called %d times, want 0", len(store.untrustCalls))
	}
	if !out.KeyExported {
		t.Errorf("KeyExported = false, want true")
	}

	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert file missing: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Errorf("key file missing: %v", err)
	}

	if state.Find("/p1", "example.test") == nil {
		t.Errorf("state.Find = nil, want entry")
	}
}

func TestApplySiteKeyMode0600(t *testing.T) {
	in, _, keyPath := applyEnv(t, "/p1", "example.test", true)

	store := &fakeStore{trustResult: TrustResult{Locations: []string{macOSLoginKeychainLocation}}}
	state := &State{}
	if out := ApplySite(store, state, in); out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}

	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat key: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("key mode = %o, want 600", got)
	}
}

func TestApplySiteFingerprintMatchAllPresentSkips(t *testing.T) {
	in, _, _ := applyEnv(t, "/p1", "example.test", false)

	fp, err := Fingerprint(in.CertPEM)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}

	state := &State{}
	state.Upsert(Entry{
		Project:     "/p1",
		Site:        "example.test",
		Fingerprint: fp,
		Locations:   []string{macOSLoginKeychainLocation},
		Label:       Label("/p1", "example.test"),
	})

	store := &fakeStore{
		verifyResult: VerifyResult{Present: []string{macOSLoginKeychainLocation}},
	}

	out := ApplySite(store, state, in)

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if !strings.HasPrefix(out.Verb, "already trusted") {
		t.Errorf("Verb = %q, want prefix %q", out.Verb, "already trusted")
	}
	if len(store.trustCalls) != 0 {
		t.Errorf("Trust called %d times, want 0", len(store.trustCalls))
	}
	if len(store.untrustCalls) != 0 {
		t.Errorf("Untrust called %d times, want 0", len(store.untrustCalls))
	}
	if len(store.verifyCalls) != 1 {
		t.Errorf("Verify called %d times, want 1", len(store.verifyCalls))
	}
}

func TestApplySiteFingerprintMatchDriftRetrusts(t *testing.T) {
	in, _, _ := applyEnv(t, "/p1", "example.test", false)

	fp, _ := Fingerprint(in.CertPEM)

	state := &State{}
	state.Upsert(Entry{
		Project:     "/p1",
		Site:        "example.test",
		Fingerprint: fp,
		Locations:   []string{macOSLoginKeychainLocation},
		Label:       Label("/p1", "example.test"),
	})

	store := &fakeStore{
		verifyResult: VerifyResult{Missing: []string{macOSLoginKeychainLocation}},
		trustResult:  TrustResult{Locations: []string{macOSLoginKeychainLocation}},
	}

	out := ApplySite(store, state, in)

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if !strings.Contains(out.Verb, "drift") {
		t.Errorf("Verb = %q, want to mention \"drift\"", out.Verb)
	}
	if len(store.untrustCalls) != 1 {
		t.Errorf("Untrust called %d times, want 1", len(store.untrustCalls))
	}
	if len(store.trustCalls) != 1 {
		t.Errorf("Trust called %d times, want 1", len(store.trustCalls))
	}
}

func TestApplySiteFingerprintChangedRetrusts(t *testing.T) {
	in, _, _ := applyEnv(t, "/p1", "example.test", false)

	state := &State{}
	state.Upsert(Entry{
		Project:     "/p1",
		Site:        "example.test",
		Fingerprint: "old-fingerprint-deadbeef",
		Locations:   []string{macOSLoginKeychainLocation},
		Label:       Label("/p1", "example.test"),
		CertPath:    "/some/old/path.cert",
	})

	store := &fakeStore{
		trustResult: TrustResult{Locations: []string{macOSLoginKeychainLocation}},
	}

	out := ApplySite(store, state, in)

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if !strings.Contains(out.Verb, "fingerprint changed") {
		t.Errorf("Verb = %q, want to mention \"fingerprint changed\"", out.Verb)
	}
	if len(store.untrustCalls) != 1 {
		t.Fatalf("Untrust called %d times, want 1", len(store.untrustCalls))
	}
	if got := store.untrustCalls[0].input.Fingerprint; got != "old-fingerprint-deadbeef" {
		t.Errorf("Untrust input fingerprint = %q, want old fingerprint", got)
	}
	if len(store.trustCalls) != 1 {
		t.Errorf("Trust called %d times, want 1", len(store.trustCalls))
	}

	// Verify state now reflects the new fingerprint.
	entry := state.Find("/p1", "example.test")
	if entry == nil {
		t.Fatalf("state.Find = nil")
	}
	if entry.Fingerprint == "old-fingerprint-deadbeef" {
		t.Errorf("state still has old fingerprint")
	}
}

func TestApplySiteUntrustErrorPreservesState(t *testing.T) {
	in, _, _ := applyEnv(t, "/p1", "example.test", false)

	state := &State{}
	state.Upsert(Entry{
		Project:     "/p1",
		Site:        "example.test",
		Fingerprint: "old",
		Locations:   []string{macOSLoginKeychainLocation},
		Label:       Label("/p1", "example.test"),
	})

	store := &fakeStore{
		untrustErr: errors.New("keychain locked"),
	}

	out := ApplySite(store, state, in)

	if out.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if out.ErrHint == "" {
		t.Errorf("ErrHint is empty, want re-run guidance")
	}
	if !strings.Contains(out.ErrHint, "--site example.test") {
		t.Errorf("ErrHint = %q, want to suggest --site example.test", out.ErrHint)
	}
	if len(store.trustCalls) != 0 {
		t.Errorf("Trust called %d times, want 0", len(store.trustCalls))
	}

	entry := state.Find("/p1", "example.test")
	if entry == nil {
		t.Fatal("state was wiped after untrust error, want preserved")
	}
	if entry.Fingerprint != "old" {
		t.Errorf("state fingerprint = %q, want preserved \"old\"", entry.Fingerprint)
	}
}

func TestApplySiteTrustErrorWithPartialLocationsPreservesState(t *testing.T) {
	in, _, _ := applyEnv(t, "/p1", "example.test", false)

	store := &fakeStore{
		trustResult: TrustResult{Locations: []string{macOSLoginKeychainLocation}},
		trustErr:    errors.New("certutil failed mid-way"),
	}
	state := &State{}

	out := ApplySite(store, state, in)

	if out.Err == nil {
		t.Fatal("Err = nil, want error")
	}

	entry := state.Find("/p1", "example.test")
	if entry == nil {
		t.Fatal("state.Find = nil; want partial entry to be recorded")
	}
	if len(entry.Locations) != 1 || entry.Locations[0] != macOSLoginKeychainLocation {
		t.Errorf("partial Locations = %v, want [macOSLoginKeychain]", entry.Locations)
	}
}

func TestApplySiteTrustErrorWithNoLocationsLeavesStateUntouched(t *testing.T) {
	in, _, _ := applyEnv(t, "/p1", "example.test", false)

	store := &fakeStore{trustErr: errors.New("certutil failed before any write")}
	state := &State{}

	out := ApplySite(store, state, in)

	if out.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if state.Find("/p1", "example.test") != nil {
		t.Errorf("state has entry, want nothing")
	}
}

func TestApplySiteEmptyCertPEMErrors(t *testing.T) {
	tmp := t.TempDir()
	in := SiteInput{
		Project:      "/p1",
		Site:         "example.test",
		InstanceName: "example.com",
		BaseDir:      tmp,
		CertPEM:      nil,
	}

	store := &fakeStore{}
	state := &State{}

	out := ApplySite(store, state, in)

	if out.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if len(store.trustCalls) != 0 {
		t.Errorf("Trust called, expected early exit")
	}
}

func TestApplySiteNoKeyClearsExistingKeyFile(t *testing.T) {
	in, _, keyPath := applyEnv(t, "/p1", "example.test", false)

	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("stale key"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := &fakeStore{trustResult: TrustResult{Locations: []string{macOSLoginKeychainLocation}}}
	state := &State{}
	if out := ApplySite(store, state, in); out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}

	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Errorf("key file still present after KeyPEM=nil; err=%v", err)
	}
	if entry := state.Find("/p1", "example.test"); entry == nil || entry.KeyPath != "" {
		t.Errorf("state.KeyPath = %q, want empty", entry.KeyPath)
	}
}

func TestApplySiteSkipPathRefreshesPaths(t *testing.T) {
	in, certPath, _ := applyEnv(t, "/p1", "example.test", false)

	fp, _ := Fingerprint(in.CertPEM)

	state := &State{}
	state.Upsert(Entry{
		Project:     "/p1",
		Site:        "example.test",
		Fingerprint: fp,
		Locations:   []string{macOSLoginKeychainLocation},
		Label:       Label("/p1", "example.test"),
		CertPath:    "/stale/path.cert",
	})

	store := &fakeStore{verifyResult: VerifyResult{Present: []string{macOSLoginKeychainLocation}}}

	if out := ApplySite(store, state, in); out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}

	entry := state.Find("/p1", "example.test")
	if entry.CertPath != certPath {
		t.Errorf("CertPath = %q, want refreshed %q", entry.CertPath, certPath)
	}
}

func TestRevokeSiteSuccess(t *testing.T) {
	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "site.cert")
	keyPath := filepath.Join(tmp, "site.key")
	if err := os.WriteFile(certPath, []byte("cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &State{}
	state.Upsert(Entry{Project: "/p1", Site: "example.test", Locations: []string{macOSLoginKeychainLocation}})

	store := &fakeStore{untrustCleaned: []string{macOSLoginKeychainLocation}}

	out := RevokeSite(store, state, "/p1", Entry{
		Project:   "/p1",
		Site:      "example.test",
		CertPath:  certPath,
		KeyPath:   keyPath,
		Locations: []string{macOSLoginKeychainLocation},
	})

	if out.Err != nil {
		t.Fatalf("Err = %v", out.Err)
	}
	if len(out.Cleaned) != 1 {
		t.Errorf("Cleaned = %v, want one location", out.Cleaned)
	}
	if _, err := os.Stat(certPath); !os.IsNotExist(err) {
		t.Errorf("cert file still present")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Errorf("key file still present")
	}
	if state.Find("/p1", "example.test") != nil {
		t.Errorf("state still has entry, want removed")
	}
}

func TestRevokeSiteUntrustErrorPreservesState(t *testing.T) {
	tmp := t.TempDir()
	certPath := filepath.Join(tmp, "site.cert")
	if err := os.WriteFile(certPath, []byte("cert"), 0o644); err != nil {
		t.Fatal(err)
	}

	state := &State{}
	state.Upsert(Entry{Project: "/p1", Site: "example.test", Locations: []string{macOSLoginKeychainLocation}})

	store := &fakeStore{untrustErr: errors.New("sudo declined")}

	out := RevokeSite(store, state, "/p1", Entry{
		Project:   "/p1",
		Site:      "example.test",
		CertPath:  certPath,
		Locations: []string{macOSLoginKeychainLocation},
	})

	if out.Err == nil {
		t.Fatal("Err = nil, want error")
	}
	if out.ErrHint == "" || !strings.Contains(out.ErrHint, "--site example.test") {
		t.Errorf("ErrHint = %q, want re-run hint", out.ErrHint)
	}
	if _, err := os.Stat(certPath); err != nil {
		t.Errorf("cert file removed despite untrust failure: %v", err)
	}
	if state.Find("/p1", "example.test") == nil {
		t.Errorf("state was wiped after untrust failure")
	}
}

func TestWriteFileAtomicSetsMode(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "secret")
	if err := writeFileAtomic(path, []byte("hunter2"), 0o600); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hunter2" {
		t.Errorf("contents = %q, want %q", string(data), "hunter2")
	}
}

func TestWriteFileAtomicReplacesExistingPermissivelyModedFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "secret")

	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600 — atomic write must not inherit prior 0o644", got)
	}
}
