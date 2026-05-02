package trust

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/roots/trellis-cli/app_paths"
)

const (
	linuxUserCALocation   = "linux-user-ca"
	linuxSystemCALocation = "linux-system-ca"
)

type linuxStore struct {
	opts Options
}

func newLinuxStore(opts Options) *linuxStore {
	return &linuxStore{opts: opts}
}

// userCADir is the per-user trust dir we manage. The tools that pick up
// trust from here do so via env vars (NODE_EXTRA_CA_CERTS, SSL_CERT_FILE,
// REQUESTS_CA_BUNDLE) pointed at the bundle file we maintain.
func userCADir() string {
	return filepath.Join(app_paths.DataDir(), "ca-certificates")
}

// userCABundle is the concatenated PEM file for tools that take a single
// bundle path. Maintained as the union of all entries in userCADir.
func userCABundle() string {
	return filepath.Join(app_paths.DataDir(), "ca-bundle.pem")
}

func (s *linuxStore) Trust(input TrustInput) (TrustResult, error) {
	result := TrustResult{}

	if err := os.MkdirAll(userCADir(), 0o755); err != nil {
		return result, err
	}

	dst := filepath.Join(userCADir(), userCAFilename(input))
	if err := os.WriteFile(dst, input.CertPEM, 0o644); err != nil {
		return result, fmt.Errorf("write user CA cert: %w", err)
	}
	if err := rebuildBundle(); err != nil {
		return result, err
	}
	result.Locations = append(result.Locations, linuxUserCALocation)

	if s.opts.TrustSystem {
		systemDst := systemCAPath(input)
		writeCmd := exec.Command("sudo", "tee", systemDst)
		writeCmd.Stdin = bytes.NewReader(input.CertPEM)
		writeCmd.Stdout = nil
		if err := writeCmd.Run(); err != nil {
			return result, fmt.Errorf("sudo tee %s: %w", systemDst, err)
		}
		// Record the location as soon as the file is on disk, so if
		// update-ca-certificates fails the caller can still track this
		// file for later cleanup via vm untrust.
		result.Locations = append(result.Locations, linuxSystemCALocation)
		if out, err := exec.Command("sudo", "update-ca-certificates").CombinedOutput(); err != nil {
			return result, fmt.Errorf("update-ca-certificates: %w: %s", err, string(out))
		}
	}

	nssLocations, nssStatus, nssErr := nssTrust(input, s.opts.DisableNSS)
	result.Locations = append(result.Locations, nssLocations...)
	result.NSS = nssStatus

	return result, nssErr
}

func (s *linuxStore) Untrust(input TrustInput, locations []string) ([]string, error) {
	var cleaned []string
	var firstErr error

	for _, loc := range locations {
		switch loc {
		case linuxUserCALocation:
			path := filepath.Join(userCADir(), userCAFilename(input))
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			cleaned = append(cleaned, loc)
		case linuxSystemCALocation:
			systemDst := systemCAPath(input)
			if err := exec.Command("sudo", "rm", "-f", systemDst).Run(); err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if out, err := exec.Command("sudo", "update-ca-certificates", "--fresh").CombinedOutput(); err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("update-ca-certificates: %w: %s", err, string(out))
				}
				continue
			}
			cleaned = append(cleaned, loc)
		}
	}

	if err := rebuildBundle(); err != nil && firstErr == nil {
		firstErr = err
	}

	nssCleaned, err := nssUntrust(input, locations)
	if err != nil && firstErr == nil {
		firstErr = err
	}
	cleaned = append(cleaned, nssCleaned...)

	return cleaned, firstErr
}

func userCAFilename(input TrustInput) string {
	return safeFilename(input.Label) + ".crt"
}

func systemCAPath(input TrustInput) string {
	return filepath.Join("/usr/local/share/ca-certificates", safeFilename(input.Label)+".crt")
}

func (s *linuxStore) Verify(input TrustInput, locations []string) (VerifyResult, error) {
	result := VerifyResult{}

	for _, loc := range locations {
		switch loc {
		case linuxUserCALocation:
			path := filepath.Join(userCADir(), userCAFilename(input))
			if !fileFingerprintMatches(path, input.Fingerprint) {
				result.Missing = append(result.Missing, loc)
				continue
			}
			// The bundle is the only thing env-var consumers read,
			// so a missing or stale bundle means user-CA trust is
			// broken even when the per-cert file is correct.
			if !bundleContainsFingerprint(input.Fingerprint) {
				result.Missing = append(result.Missing, loc)
				continue
			}
			result.Present = append(result.Present, loc)
		case linuxSystemCALocation:
			path := systemCAPath(input)
			if !fileFingerprintMatches(path, input.Fingerprint) {
				result.Missing = append(result.Missing, loc)
				continue
			}
			// File on disk is necessary but not sufficient: trust
			// only takes effect once the system bundle has been
			// regenerated. A previous update-ca-certificates that
			// failed leaves the source file behind but no live trust.
			if !systemBundleContainsFingerprint(input.Fingerprint) {
				result.Missing = append(result.Missing, loc)
				continue
			}
			result.Present = append(result.Present, loc)
		}
	}

	nssPresent, nssMissing, nssUnknown := nssVerify(input, locations)
	result.Present = append(result.Present, nssPresent...)
	result.Missing = append(result.Missing, nssMissing...)
	result.Unknown = append(result.Unknown, nssUnknown...)

	return result, nil
}

func fileFingerprintMatches(path, fingerprint string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	fp, err := Fingerprint(data)
	if err != nil {
		return false
	}
	return fp == fingerprint
}

func bundleContainsFingerprint(fingerprint string) bool {
	return pemFileContainsFingerprint(userCABundle(), fingerprint)
}

// systemBundleContainsFingerprint walks the system trust bundle managed
// by update-ca-certificates and reports whether the fingerprint is present.
// Only the Debian/Ubuntu path is checked because that's the install flow
// our Trust path uses (/usr/local/share/ca-certificates + update-ca-
// certificates). Verifying against a RHEL/Fedora bundle would imply
// support we don't actually provide.
func systemBundleContainsFingerprint(fingerprint string) bool {
	return pemFileContainsFingerprint("/etc/ssl/certs/ca-certificates.crt", fingerprint)
}

func pemFileContainsFingerprint(path, fingerprint string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	rest := data
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			return false
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		sum := sha256.Sum256(block.Bytes)
		if hex.EncodeToString(sum[:]) == fingerprint {
			return true
		}
	}
}

func rebuildBundle() error {
	entries, err := os.ReadDir(userCADir())
	if err != nil {
		if os.IsNotExist(err) {
			return os.Remove(userCABundle())
		}
		return err
	}

	var combined []byte
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(userCADir(), entry.Name()))
		if err != nil {
			return err
		}
		combined = append(combined, data...)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			combined = append(combined, '\n')
		}
	}

	bundlePath := userCABundle()

	if len(combined) == 0 {
		_ = os.Remove(bundlePath)
		return nil
	}

	dir := filepath.Dir(bundlePath)
	tmp, err := os.CreateTemp(dir, ".ca-bundle.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(combined); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, bundlePath)
}

func safeFilename(label string) string {
	out := make([]byte, 0, len(label))
	for i := 0; i < len(label); i++ {
		c := label[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
