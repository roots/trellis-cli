package trust

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SiteInput describes a single site's cert (and optionally key) bytes that
// ApplySite should export to disk and add to the host's trust stores.
type SiteInput struct {
	Project      string
	Site         string
	InstanceName string
	BaseDir      string
	CertPEM      []byte
	// KeyPEM, when non-empty, is written to <ExportDir>/<Site>.key with 0o600.
	// When empty, any previously exported key file at that path is removed.
	KeyPEM []byte
}

// SiteOutcome is the per-site result of ApplySite. The cmd layer maps Verb
// to a one-line user-facing message and surfaces Err / ErrHint on failure.
type SiteOutcome struct {
	Site        string
	Verb        string
	Locations   []string
	NSS         NSSStatus
	KeyExported bool
	Err         error
	ErrHint     string
}

// ApplySite exports the cert and key (if provided) for a single site to the
// project's export dir, then trusts the cert in the host's stores. State is
// updated in place; callers are responsible for State.Save() at the end of a
// batch.
//
// On a fingerprint match it verifies the live trust setting and re-trusts
// only if drift is detected. On a fingerprint change it removes the previous
// trust entries before adding the new one.
func ApplySite(store Store, state *State, in SiteInput) SiteOutcome {
	out := SiteOutcome{Site: in.Site}

	exportDir := ExportDir(in.BaseDir, in.InstanceName, in.Project)
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		out.Err = fmt.Errorf("create export directory: %w", err)
		return out
	}
	certPath := filepath.Join(exportDir, in.Site+".cert")
	keyPath := filepath.Join(exportDir, in.Site+".key")

	if len(in.CertPEM) == 0 {
		out.Err = fmt.Errorf("VM returned an empty cert. Has the site been provisioned with ssl.enabled?")
		return out
	}
	if err := os.WriteFile(certPath, in.CertPEM, 0o644); err != nil {
		out.Err = fmt.Errorf("write cert to host: %w", err)
		return out
	}

	if len(in.KeyPEM) > 0 {
		if err := writeFileAtomic(keyPath, in.KeyPEM, 0o600); err != nil {
			out.Err = fmt.Errorf("write private key to host: %w", err)
			return out
		}
		out.KeyExported = true
	} else {
		// Clear any previously exported key so an empty KeyPEM is observably
		// honored even on fingerprint match.
		_ = os.Remove(keyPath)
	}

	fingerprint, err := Fingerprint(in.CertPEM)
	if err != nil {
		out.Err = fmt.Errorf("fingerprint cert: %w", err)
		return out
	}
	commonName, _ := CommonName(in.CertPEM)
	label := Label(in.Project, in.Site)

	input := TrustInput{
		CertPath:    certPath,
		CertPEM:     in.CertPEM,
		Fingerprint: fingerprint,
		Label:       label,
	}

	existing := state.Find(in.Project, in.Site)
	retrustReason := ""
	if existing != nil && existing.Fingerprint == fingerprint {
		verifyInput := input
		verifyInput.Label = existing.Label
		verify, _ := store.Verify(verifyInput, existing.Locations)
		if verify.AllAccounted(existing.Locations) {
			existing.CertPath = certPath
			existing.KeyPath = keyPathOrEmpty(keyPath, out.KeyExported)
			state.Upsert(*existing)
			out.Verb = "already trusted (fingerprint match, skipped)"
			out.Locations = existing.Locations
			return out
		}
		retrustReason = fmt.Sprintf("drift, %d location(s) missing", len(verify.Missing))
	} else if existing != nil {
		retrustReason = "fingerprint changed"
	}

	if existing != nil {
		oldInput := TrustInput{
			CertPath:    existing.CertPath,
			Fingerprint: existing.Fingerprint,
			Label:       existing.Label,
		}
		if _, err := store.Untrust(oldInput, existing.Locations); err != nil {
			out.Err = fmt.Errorf("remove previous trust entry: %w", err)
			out.ErrHint = fmt.Sprintf("state preserved; resolve the underlying issue and re-run `trellis vm trust --site %s`", in.Site)
			return out
		}
		state.Remove(in.Project, in.Site)
	}

	result, trustErr := store.Trust(input)
	out.Locations = result.Locations
	out.NSS = result.NSS

	entry := Entry{
		Project:     in.Project,
		Site:        in.Site,
		Fingerprint: fingerprint,
		CommonName:  commonName,
		CertPath:    certPath,
		KeyPath:     keyPathOrEmpty(keyPath, out.KeyExported),
		Label:       label,
		Locations:   result.Locations,
		AddedAt:     time.Now().UTC(),
	}

	if trustErr != nil {
		// Preserve whatever locations did get applied so a later untrust
		// can clean them up.
		if len(result.Locations) > 0 {
			state.Upsert(entry)
		}
		out.Err = trustErr
		return out
	}

	state.Upsert(entry)
	if retrustReason != "" {
		out.Verb = fmt.Sprintf("re-trusted (%s)", retrustReason)
	} else {
		out.Verb = "trusted"
	}
	return out
}

// RevokeOutcome is the per-site result of RevokeSite.
type RevokeOutcome struct {
	Site    string
	Cleaned []string
	Err     error
	ErrHint string
}

// RevokeSite removes one site's trust entry from the host stores and drops
// its exported cert+key files. State is updated in place on success;
// callers are responsible for State.Save() at the end of a batch.
func RevokeSite(store Store, state *State, project string, entry Entry) RevokeOutcome {
	out := RevokeOutcome{Site: entry.Site}

	input := TrustInput{
		CertPath:    entry.CertPath,
		Fingerprint: entry.Fingerprint,
		Label:       entry.Label,
	}

	cleaned, err := store.Untrust(input, entry.Locations)
	if err != nil {
		out.Err = err
		out.ErrHint = fmt.Sprintf("state preserved so you can re-run `trellis vm untrust --site %s`", entry.Site)
		return out
	}
	out.Cleaned = cleaned

	if entry.CertPath != "" {
		_ = os.Remove(entry.CertPath)
	}
	if entry.KeyPath != "" {
		_ = os.Remove(entry.KeyPath)
	}
	state.Remove(project, entry.Site)
	return out
}

func keyPathOrEmpty(path string, exported bool) string {
	if !exported {
		return ""
	}
	return path
}

// writeFileAtomic writes data to path via a temp file in the same directory,
// then renames into place. The temp file is created with the requested mode
// so the final file never has a wider mode than asked, even briefly.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
