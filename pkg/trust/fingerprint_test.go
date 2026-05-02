package trust

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func generateTestCertPEM(t *testing.T, cn string) ([]byte, []byte) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	return pemBytes, der
}

func TestFingerprintMatchesSHA256OfDER(t *testing.T) {
	pemBytes, der := generateTestCertPEM(t, "example.test")

	got, err := Fingerprint(pemBytes)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}

	expected := sha256.Sum256(der)
	want := hex.EncodeToString(expected[:])

	if got != want {
		t.Errorf("Fingerprint = %q, want %q", got, want)
	}
}

func TestFingerprintSHA1IsUppercaseHex(t *testing.T) {
	pemBytes, der := generateTestCertPEM(t, "example.test")

	got, err := FingerprintSHA1(pemBytes)
	if err != nil {
		t.Fatalf("FingerprintSHA1: %v", err)
	}

	expected := sha1.Sum(der)
	want := strings.ToUpper(hex.EncodeToString(expected[:]))

	if got != want {
		t.Errorf("FingerprintSHA1 = %q, want %q", got, want)
	}
	if got != strings.ToUpper(got) {
		t.Errorf("expected uppercase hex, got %q", got)
	}
}

func TestCommonName(t *testing.T) {
	pemBytes, _ := generateTestCertPEM(t, "fixtures.test")

	cn, err := CommonName(pemBytes)
	if err != nil {
		t.Fatalf("CommonName: %v", err)
	}
	if cn != "fixtures.test" {
		t.Errorf("CommonName = %q, want %q", cn, "fixtures.test")
	}
}

func TestFingerprintRejectsNonPEM(t *testing.T) {
	if _, err := Fingerprint([]byte("not a pem cert")); err == nil {
		t.Fatal("expected error for non-PEM input, got nil")
	}
}
