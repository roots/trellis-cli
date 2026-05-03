package trust

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
)

// ProjectID is a short stable identifier derived from the absolute project
// path. It scopes trust labels and filenames so two projects with the same
// site name don't collide in the user's keychain or NSS DBs.
func ProjectID(projectPath string) string {
	hash := sha256.Sum256([]byte(projectPath))
	return hex.EncodeToString(hash[:4])
}

// Label returns the label used to identify a site's cert in trust stores
// (macOS keychain via cert subject, NSS nicknames, Linux user CA filenames).
// Format: trellis-<projectID>-<site>.
func Label(projectPath, siteName string) string {
	return fmt.Sprintf("trellis-%s-%s", ProjectID(projectPath), siteName)
}

// ExportDir returns the per-project directory where exported cert and key
// files live. The directory is keyed by VM instance name (which defaults to
// the main site key) plus a short hash of the absolute project path so two
// forks of the same project don't collide.
func ExportDir(baseDir, instanceName, projectPath string) string {
	return filepath.Join(baseDir, "ssl", instanceName+"-"+ProjectID(projectPath))
}
