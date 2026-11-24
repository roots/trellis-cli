package doctor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/roots/trellis-cli/github"
	"github.com/roots/trellis-cli/update"
)

const trellisRepo = "roots/trellis"

// TrellisRelease checks whether the project's Trellis (its VERSION file,
// e.g. "v1.33.0"; "" if missing) is the latest release. The latest release
// is cached in cacheDir for a day.
func TrellisRelease(current, cacheDir string, offline bool) Result {
	r := Result{ID: "trellis-version", Title: "Trellis"}

	if current == "" {
		r.Status, r.Message = StatusSkip, "version unknown (no VERSION file)"
		r.Details = []string{"https://github.com/roots/trellis/releases"}
		return r
	}

	if _, err := version.NewVersion(current); err != nil {
		r.Status, r.Message = StatusSkip, current+" (not a release)"
		return r
	}

	if offline {
		r.Status, r.Message = StatusInfo, current
		return r
	}

	notifier := &update.Notifier{
		CacheDir:   filepath.Join(cacheDir, "trellis"),
		ForceCheck: true,
		Repo:       trellisRepo,
		Version:    current,
		Client:     github.Client,
	}

	release, err := notifier.CheckForUpdate()
	switch {
	case err != nil:
		// Being behind is at worst a warning, so a failed lookup isn't
		// treated as unknown. GitHub's error bodies are long JSON, and a rate
		// limit message includes the caller's IP address.
		r.Status, r.Message = StatusInfo, current+" (couldn't check for the latest release)"
		r.Details = []string{"GitHub API request failed"}
		if strings.Contains(err.Error(), "rate limit") {
			r.Details = []string{"GitHub API rate limit exceeded; try again later or use --offline"}
		}
	case release != nil:
		r.Status, r.Message = StatusWarn, fmt.Sprintf("%s → %s available", current, release.Version)
		r.Details = []string{fmt.Sprintf("https://github.com/%s/compare/%s...%s", trellisRepo, current, release.Version)}
	default:
		r.Status, r.Message = StatusOK, current+" (latest)"
	}

	return r
}
