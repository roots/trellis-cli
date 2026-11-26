package engine

import (
	"encoding/base64"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestKeyFingerprintOfSuryKey(t *testing.T) {
	data, err := os.ReadFile("../host/hosttest/testdata/sury-php.gpg")
	if err != nil {
		t.Fatal(err)
	}
	fprs, err := keyFingerprints(data)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(fprs, "15058500A0235D97F5D10063B188E2B695BD4743") {
		t.Errorf("fingerprints = %v", fprs)
	}
}

func TestKeyFingerprintsRejectsGarbage(t *testing.T) {
	for _, in := range []string{"content of https://example.com/key", "", "\x99\xff"} {
		if _, err := keyFingerprints([]byte(in)); err == nil {
			t.Errorf("accepted %q", in)
		}
	}
}

func TestKeyFingerprintArmored(t *testing.T) {
	data, err := os.ReadFile("../host/hosttest/testdata/sury-php.gpg")
	if err != nil {
		t.Fatal(err)
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	var armored strings.Builder
	armored.WriteString("-----BEGIN PGP PUBLIC KEY BLOCK-----\nComment: test\n\n")
	for len(b64) > 64 {
		armored.WriteString(b64[:64] + "\n")
		b64 = b64[64:]
	}
	armored.WriteString(b64 + "\n=abcd\n-----END PGP PUBLIC KEY BLOCK-----\n")
	fprs, err := keyFingerprints([]byte(armored.String()))
	if err != nil || !slices.Contains(fprs, "15058500A0235D97F5D10063B188E2B695BD4743") {
		t.Fatalf("armored: %v %v", fprs, err)
	}
}
