package trust

import (
	"fmt"
	"runtime"
)

// TrustInput describes a cert that should be added to the host's trust stores.
type TrustInput struct {
	// CertPath is an absolute path to the PEM-encoded certificate already
	// present on disk. Stores that need a file (e.g. macOS `security`,
	// `certutil -i`) read it from here.
	CertPath string
	// CertPEM is the same content as the file at CertPath. Provided so
	// callers don't have to re-read the file.
	CertPEM []byte
	// Fingerprint is the hex-encoded SHA-256 of the DER bytes; used as a
	// stable identifier for state tracking and verification.
	Fingerprint string
	// Label is a human-readable name used for keychain entries and NSS
	// nicknames, e.g. "trellis: example.com".
	Label string
}

// TrustResult bundles what a Trust call did across all underlying stores.
type TrustResult struct {
	Locations []string
	NSS       NSSStatus
}

// VerifyResult classifies each recorded location as present, missing, or
// unknown. Unknown means the verifier couldn't determine the state (e.g.
// certutil isn't on PATH so NSS can't be queried) and should be treated
// as not-yet-broken so we don't accidentally re-trust and lose state.
type VerifyResult struct {
	Present []string
	Missing []string
	Unknown []string
}

// AllAccounted reports whether every recorded location was either confirmed
// present or could not be checked. If anything is Missing, the caller
// should treat the entry as drifted and re-trust.
func (v VerifyResult) AllAccounted(locations []string) bool {
	return len(v.Missing) == 0 && len(v.Present)+len(v.Unknown) == len(locations)
}

// Store applies trust changes for the cert across the platform's trust
// stores. Each implementation may write to multiple underlying stores
// (system keychain + NSS, etc.); TrustResult captures all of them so the
// caller can record state and surface NSS hints.
type Store interface {
	// Trust adds the cert. Idempotent: re-running against an
	// already-trusted cert should succeed without error.
	Trust(input TrustInput) (TrustResult, error)
	// Untrust removes the cert from the locations recorded at trust time.
	// It returns the list of locations actually cleaned. Missing entries
	// are skipped silently.
	Untrust(input TrustInput, locations []string) ([]string, error)
	// Verify classifies each recorded location as present, missing, or
	// unknown. Used to detect drift between recorded state and reality
	// (e.g. user manually deleted from keychain).
	Verify(input TrustInput, locations []string) (VerifyResult, error)
}

// Options influence how Default constructs the platform store.
type Options struct {
	// TrustSystem (Linux) toggles writing to /usr/local/share/ca-certificates
	// and running `sudo update-ca-certificates`. Off by default because it
	// requires sudo.
	TrustSystem bool
	// DisableNSS skips Firefox NSS profile updates even when certutil is
	// available. Mostly useful for tests.
	DisableNSS bool
}

// Default returns the trust store appropriate for the current host.
func Default(opts Options) (Store, error) {
	switch runtime.GOOS {
	case "darwin":
		return newMacOSStore(opts), nil
	case "linux":
		return newLinuxStore(opts), nil
	default:
		return nil, fmt.Errorf("trust store not implemented for %s", runtime.GOOS)
	}
}
