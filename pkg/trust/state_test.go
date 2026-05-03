package trust

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStateUpsertAndFind(t *testing.T) {
	s := &State{}

	entry := Entry{
		Project:     "/projects/site-a",
		Site:        "example.com",
		Fingerprint: "abc",
		Locations:   []string{macOSLoginKeychainLocation},
		AddedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	s.Upsert(entry)

	if got := s.Find("/projects/site-a", "example.com"); got == nil || got.Fingerprint != "abc" {
		t.Fatalf("Find after Upsert returned %#v, want fingerprint abc", got)
	}

	updated := entry
	updated.Fingerprint = "def"
	s.Upsert(updated)

	got := s.Find("/projects/site-a", "example.com")
	if got == nil || got.Fingerprint != "def" {
		t.Fatalf("Upsert did not replace existing entry: got %#v", got)
	}
	if len(s.Entries) != 1 {
		t.Fatalf("expected 1 entry after replacement, got %d", len(s.Entries))
	}
}

func TestStateRemoveOnlyTargetsProjectAndSite(t *testing.T) {
	s := &State{}
	s.Upsert(Entry{Project: "p1", Site: "a.test", Fingerprint: "1"})
	s.Upsert(Entry{Project: "p1", Site: "b.test", Fingerprint: "2"})
	s.Upsert(Entry{Project: "p2", Site: "a.test", Fingerprint: "3"})

	s.Remove("p1", "a.test")

	if s.Find("p1", "a.test") != nil {
		t.Fatal("expected p1/a.test removed")
	}
	if s.Find("p1", "b.test") == nil {
		t.Fatal("expected p1/b.test preserved")
	}
	if s.Find("p2", "a.test") == nil {
		t.Fatal("expected p2/a.test preserved")
	}
}

func TestEntriesForProjectIsSortedBySite(t *testing.T) {
	s := &State{}
	s.Upsert(Entry{Project: "p1", Site: "z.test", Fingerprint: "1"})
	s.Upsert(Entry{Project: "p1", Site: "a.test", Fingerprint: "2"})
	s.Upsert(Entry{Project: "p2", Site: "m.test", Fingerprint: "3"})

	entries := s.EntriesForProject("p1")

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for p1, got %d", len(entries))
	}
	if entries[0].Site != "a.test" || entries[1].Site != "z.test" {
		t.Fatalf("expected [a.test, z.test], got [%s, %s]", entries[0].Site, entries[1].Site)
	}
}

// withDataDir points app_paths.DataDir at a tempdir for the test's lifetime.
func withDataDir(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	return filepath.Join(tmp, "trellis")
}

func TestLoadReturnsEmptyWhenStateFileMissing(t *testing.T) {
	withDataDir(t)

	state, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Entries) != 0 {
		t.Errorf("Entries = %d, want 0", len(state.Entries))
	}
}

func TestLoadReturnsEmptyWhenStateFileEmpty(t *testing.T) {
	withDataDir(t)

	if err := os.MkdirAll(filepath.Dir(StatePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StatePath(), nil, 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(state.Entries) != 0 {
		t.Errorf("Entries = %d, want 0", len(state.Entries))
	}
}

func TestLoadFailsOnCorruptJSON(t *testing.T) {
	withDataDir(t)

	if err := os.MkdirAll(filepath.Dir(StatePath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(StatePath(), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(); err == nil {
		t.Fatal("Load() = nil err on corrupt JSON; want error")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withDataDir(t)

	original := &State{}
	original.Upsert(Entry{
		Project:     "/p1",
		Site:        "example.test",
		Fingerprint: "abc",
		CommonName:  "example.test",
		CertPath:    "/data/ssl/example.test.cert",
		Label:       "trellis-deadbeef-example.test",
		Locations:   []string{macOSLoginKeychainLocation},
		AddedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})

	if err := original.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(loaded.Entries))
	}
	got := loaded.Entries[0]
	want := original.Entries[0]
	if got.Project != want.Project || got.Site != want.Site || got.Fingerprint != want.Fingerprint {
		t.Errorf("round-trip mismatch:\n got %+v\nwant %+v", got, want)
	}
	if !got.AddedAt.Equal(want.AddedAt) {
		t.Errorf("AddedAt round-trip: got %v, want %v", got.AddedAt, want.AddedAt)
	}
}

func TestSaveLeavesNoTempFilesBehind(t *testing.T) {
	withDataDir(t)

	s := &State{}
	s.Upsert(Entry{Project: "/p1", Site: "a.test", Fingerprint: "x"})
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dir := filepath.Dir(StatePath())
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		name := e.Name()
		if name == filepath.Base(StatePath()) {
			continue
		}
		t.Errorf("unexpected leftover file in state dir: %q", name)
	}
}
