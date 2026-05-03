package trust

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
