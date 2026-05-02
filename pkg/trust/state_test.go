package trust

import (
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
