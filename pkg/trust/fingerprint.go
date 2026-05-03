package trust

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
)

// Fingerprint returns the hex-encoded SHA-256 fingerprint of the DER bytes
// of the first certificate in a PEM-encoded byte slice.
func Fingerprint(certPEM []byte) (string, error) {
	der, err := derFromPEM(certPEM)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), nil
}

// CommonName returns the Subject CN of the first certificate in the PEM bytes.
func CommonName(certPEM []byte) (string, error) {
	der, err := derFromPEM(certPEM)
	if err != nil {
		return "", err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return "", fmt.Errorf("parse certificate: %w", err)
	}
	return cert.Subject.CommonName, nil
}

func derFromPEM(certPEM []byte) ([]byte, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found in certificate data")
	}
	return block.Bytes, nil
}
