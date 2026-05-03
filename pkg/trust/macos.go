package trust

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const macOSLoginKeychainLocation = "macos-login-keychain"

type macOSStore struct {
	opts   Options
	runner runner
	nss    nssHelper
}

func newMacOSStore(opts Options) *macOSStore {
	r := runner(execRunner{})
	return &macOSStore{opts: opts, runner: r, nss: nssHelper{runner: r}}
}

func (s *macOSStore) Trust(input TrustInput) (TrustResult, error) {
	result := TrustResult{}

	keychain, err := loginKeychainPath()
	if err != nil {
		return result, err
	}

	// `-p ssl` constrains the trust to TLS server validation rather than
	// every X.509 use case (S/MIME, code signing, etc.). `-r trustRoot`
	// makes the self-signed leaf its own trust anchor.
	if _, combined, err := s.runner.Run("security", "add-trusted-cert",
		"-r", "trustRoot",
		"-p", "ssl",
		"-k", keychain,
		input.CertPath,
	); err != nil {
		return result, fmt.Errorf("security add-trusted-cert: %w: %s", err, string(combined))
	}
	result.Locations = []string{macOSLoginKeychainLocation}

	nssLocations, nssStatus, nssErr := s.nss.trust(input, s.opts.DisableNSS)
	result.Locations = append(result.Locations, nssLocations...)
	result.NSS = nssStatus

	return result, nssErr
}

func (s *macOSStore) Untrust(input TrustInput, locations []string) ([]string, error) {
	var cleaned []string
	var firstErr error

	for _, loc := range locations {
		if loc != macOSLoginKeychainLocation {
			continue
		}

		// Drop the trust setting first; ignore if not present.
		_, _, _ = s.runner.Run("security", "remove-trusted-cert", input.CertPath)

		sha1Hex := sha1HexFromCertFile(input.CertPath)

		// Skip the delete entirely when the cert isn't in the keychain.
		// The Store contract says missing entries are not errors.
		if s.keychainHasFingerprint(sha1Hex) == keychainMissing {
			cleaned = append(cleaned, loc)
			continue
		}

		if sha1Hex != "" {
			if _, combined, err := s.runner.Run("security", "delete-certificate", "-Z", sha1Hex); err != nil {
				if isKeychainNotFound(string(combined)) {
					cleaned = append(cleaned, loc)
					continue
				}
				if firstErr == nil {
					firstErr = fmt.Errorf("security delete-certificate: %w: %s", err, string(combined))
				}
				continue
			}
		}
		cleaned = append(cleaned, loc)
	}

	nssCleaned, err := s.nss.untrust(input, locations)
	if err != nil && firstErr == nil {
		firstErr = err
	}
	cleaned = append(cleaned, nssCleaned...)

	return cleaned, firstErr
}

func isKeychainNotFound(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "could not be found") ||
		strings.Contains(s, "specified item could not be found") ||
		strings.Contains(s, "no matching items")
}

func (s *macOSStore) Verify(input TrustInput, locations []string) (VerifyResult, error) {
	result := VerifyResult{}

	for _, loc := range locations {
		if loc != macOSLoginKeychainLocation {
			continue
		}
		state := s.keychainTrustsCertForSSL(input.CertPath)
		switch state {
		case keychainPresent:
			result.Present = append(result.Present, loc)
		case keychainMissing:
			result.Missing = append(result.Missing, loc)
		default:
			result.Unknown = append(result.Unknown, loc)
		}
	}

	nssPresent, nssMissing, nssUnknown := s.nss.verify(input, locations)
	result.Present = append(result.Present, nssPresent...)
	result.Missing = append(result.Missing, nssMissing...)
	result.Unknown = append(result.Unknown, nssUnknown...)

	return result, nil
}

type keychainState int

const (
	keychainUnknown keychainState = iota
	keychainPresent
	keychainMissing
)

// keychainTrustsCertForSSL checks both that the cert exists in a keychain
// AND that current trust settings make it valid for SSL — `security
// verify-cert -p ssl` returns 0 only when the chain validates and the
// trust override is still in place. A user manually removing the trust
// override (but leaving the cert in the keychain) returns Missing here so
// re-trust will reapply the override.
func (s *macOSStore) keychainTrustsCertForSSL(certPath string) keychainState {
	if certPath == "" {
		return keychainUnknown
	}
	if _, err := os.Stat(certPath); err != nil {
		return keychainUnknown
	}
	keychain, err := loginKeychainPath()
	if err != nil {
		return keychainUnknown
	}
	// `-k` pins verification to the same keychain Trust writes into,
	// matching the rest of the file rather than relying on the user's
	// search list.
	_, combined, err := s.runner.Run("security", "verify-cert",
		"-c", certPath,
		"-p", "ssl",
		"-k", keychain,
	)
	if err == nil {
		return keychainPresent
	}
	// Distinguish "trust missing" from genuine exec errors. verify-cert
	// emits a clear message ("CSSMERR_TP_NOT_TRUSTED" / "not trusted")
	// for the trust-failure path; treat anything else as Unknown.
	msg := strings.ToLower(string(combined))
	if strings.Contains(msg, "not trusted") ||
		strings.Contains(msg, "tp_not_trusted") ||
		strings.Contains(msg, "tp_invalid_anchor_cert") ||
		strings.Contains(msg, "cert verify result: false") {
		return keychainMissing
	}
	return keychainUnknown
}

// keychainHasFingerprint scans the user's login keychain for any cert whose
// SHA-1 matches. Used by Untrust to skip delete-certificate when the cert
// is already gone.
func (s *macOSStore) keychainHasFingerprint(sha1Hex string) keychainState {
	if sha1Hex == "" {
		return keychainUnknown
	}
	keychain, err := loginKeychainPath()
	if err != nil {
		return keychainUnknown
	}
	stdout, _, err := s.runner.Run("security", "find-certificate", "-a", "-Z", keychain)
	if err != nil {
		return keychainUnknown
	}
	needle := "SHA-1 hash: " + strings.ToUpper(sha1Hex)
	if strings.Contains(string(stdout), needle) {
		return keychainPresent
	}
	return keychainMissing
}

// sha1HexFromCertFile reads a PEM cert from disk and returns the uppercase
// SHA-1 hex of its DER bytes — the form `security delete-certificate -Z`
// expects. Returns "" on any error so callers can fall back to a no-op.
func sha1HexFromCertFile(path string) string {
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	der, err := derFromPEM(data)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(der)
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func loginKeychainPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "Keychains", "login.keychain-db"), nil
}
