package trust

import (
	"strings"
	"testing"
)

func TestProjectIDIsStable(t *testing.T) {
	got1 := ProjectID("/a/b/c")
	got2 := ProjectID("/a/b/c")
	if got1 != got2 {
		t.Fatalf("ProjectID not stable across calls: %q vs %q", got1, got2)
	}
	if len(got1) != 8 {
		t.Errorf("ProjectID = %q, want length 8 (sha256[:4] hex)", got1)
	}
}

func TestProjectIDDistinguishesPaths(t *testing.T) {
	a := ProjectID("/projects/site-a")
	b := ProjectID("/projects/site-b")
	if a == b {
		t.Fatalf("ProjectID collided for distinct paths: both %q", a)
	}
}

func TestLabelFormat(t *testing.T) {
	got := Label("/projects/site-a", "example.com")
	if !strings.HasPrefix(got, "trellis-") {
		t.Errorf("Label = %q, want prefix \"trellis-\"", got)
	}
	if !strings.HasSuffix(got, "-example.com") {
		t.Errorf("Label = %q, want suffix \"-example.com\"", got)
	}
}

func TestLabelStableAcrossCalls(t *testing.T) {
	a := Label("/projects/x", "site.test")
	b := Label("/projects/x", "site.test")
	if a != b {
		t.Errorf("Label not stable: %q vs %q", a, b)
	}
}

func TestExportDirIncludesInstanceAndProjectID(t *testing.T) {
	got := ExportDir("/data", "example.com", "/projects/site-a")
	id := ProjectID("/projects/site-a")
	want := "/data/ssl/example.com-" + id
	if got != want {
		t.Errorf("ExportDir = %q, want %q", got, want)
	}
}

func TestExportDirSeparatesProjectsWithSameInstance(t *testing.T) {
	a := ExportDir("/data", "example.com", "/projects/fork-a")
	b := ExportDir("/data", "example.com", "/projects/fork-b")
	if a == b {
		t.Fatalf("ExportDir collided across forks: both %q", a)
	}
}
