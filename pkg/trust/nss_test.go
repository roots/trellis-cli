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
