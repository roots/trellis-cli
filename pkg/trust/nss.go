package trust

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// NSSStatus reports the outcome of the Firefox/NSS trust attempt so callers
// can decide whether to print install hints.
type NSSStatus struct {
	FirefoxFound    bool
	CertutilMissing bool
}

// nssTrust attempts to add the cert to every Firefox profile NSS database
// it can find. Returns the list of profile paths that were updated, plus a
// status describing whether Firefox was present and whether certutil was
// available. A missing certutil binary is not a failure — it surfaces in
// the status so the caller can prompt the user to install nss tools.
func nssTrust(input TrustInput, disabled bool) ([]string, NSSStatus, error) {
	status := NSSStatus{}
	if disabled {
		return nil, status, nil
	}

	profiles := firefoxProfileDirs()
	status.FirefoxFound = len(profiles) > 0

	certutil, err := exec.LookPath("certutil")
	if err != nil {
		status.CertutilMissing = true
		return nil, status, nil
	}

	if !status.FirefoxFound {
		return nil, status, nil
	}

	var locations []string
	for _, profile := range profiles {
		// Re-trust: drop any existing entry under our nickname so the
		// fresh cert replaces it cleanly.
		_ = exec.Command(certutil, "-D", "-d", "sql:"+profile, "-n", input.Label).Run()

		// `C,,` makes the cert a trusted SSL root — required so NSS
		// accepts a self-signed leaf as its own trust anchor. `P,,`
		// (trusted peer) is not sufficient for Firefox.
		cmd := exec.Command(certutil, "-A", "-d", "sql:"+profile,
			"-n", input.Label, "-i", input.CertPath, "-t", "C,,")
		if out, err := cmd.CombinedOutput(); err != nil {
			return locations, status, fmt.Errorf("certutil add to %s: %w: %s", profile, err, string(out))
		}
		locations = append(locations, "nss:"+profile)
	}
	return locations, status, nil
}

func nssUntrust(input TrustInput, locations []string) ([]string, error) {
	const prefix = "nss:"

	hasNSSRecord := false
	for _, loc := range locations {
		if len(loc) > len(prefix) && loc[:len(prefix)] == prefix {
			hasNSSRecord = true
			break
		}
	}

	certutil, err := exec.LookPath("certutil")
	if err != nil {
		if hasNSSRecord {
			return nil, fmt.Errorf("certutil not on PATH but recorded NSS locations exist; install nss / libnss3-tools and re-run")
		}
		return nil, nil
	}

	var cleaned []string
	var firstErr error
	for _, loc := range locations {
		if len(loc) <= len(prefix) || loc[:len(prefix)] != prefix {
			continue
		}
		profile := loc[len(prefix):]
		out, err := exec.Command(certutil, "-D", "-d", "sql:"+profile, "-n", input.Label).CombinedOutput()
		if err != nil {
			if isNSSNicknameNotFound(string(out)) {
				cleaned = append(cleaned, loc)
				continue
			}
			if firstErr == nil {
				firstErr = fmt.Errorf("certutil -D %s: %w: %s", profile, err, strings.TrimSpace(string(out)))
			}
			continue
		}
		cleaned = append(cleaned, loc)
	}
	return cleaned, firstErr
}

func isNSSNicknameNotFound(output string) bool {
	s := strings.ToLower(output)
	return strings.Contains(s, "could not find") ||
		strings.Contains(s, "no such certificate") ||
		strings.Contains(s, "sec_error_unrecognized_oid")
}

// nssVerify classifies each NSS location as present (cert with matching
// fingerprint stored under our label), missing (label not found, or stored
// under a different fingerprint), or unknown (certutil not on PATH so we
// cannot query NSS at all).
func nssVerify(input TrustInput, locations []string) (present, missing, unknown []string) {
	const prefix = "nss:"

	certutil, err := exec.LookPath("certutil")
	if err != nil {
		for _, loc := range locations {
			if len(loc) > len(prefix) && loc[:len(prefix)] == prefix {
				unknown = append(unknown, loc)
			}
		}
		return present, missing, unknown
	}

	for _, loc := range locations {
		if len(loc) <= len(prefix) || loc[:len(prefix)] != prefix {
			continue
		}
		profile := loc[len(prefix):]
		out, err := exec.Command(certutil, "-L", "-d", "sql:"+profile, "-n", input.Label, "-a").Output()
		if err != nil {
			missing = append(missing, loc)
			continue
		}
		fp, err := Fingerprint(out)
		if err != nil || fp != input.Fingerprint {
			missing = append(missing, loc)
			continue
		}
		present = append(present, loc)
	}
	return present, missing, unknown
}

// firefoxProfileDirs returns paths of likely Firefox NSS profile dirs. We
// glob known parent dirs and filter to those that look like an NSS DB.
func firefoxProfileDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	var roots []string
	switch runtime.GOOS {
	case "darwin":
		roots = []string{
			filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles"),
		}
	default:
		roots = []string{
			filepath.Join(home, ".mozilla", "firefox"),
			filepath.Join(home, "snap", "firefox", "common", ".mozilla", "firefox"),
		}
	}

	var profiles []string
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			profile := filepath.Join(root, entry.Name())
			if isNSSProfile(profile) {
				profiles = append(profiles, profile)
			}
		}
	}
	return profiles
}

func isNSSProfile(dir string) bool {
	for _, marker := range []string{"cert9.db", "cert8.db"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}
