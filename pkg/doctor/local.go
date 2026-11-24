package doctor

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/roots/trellis-cli/github"
	"github.com/roots/trellis-cli/trellis"
	"github.com/roots/trellis-cli/update"
)

const trellisRepo = "roots/trellis"

type LocalOptions struct {
	// CLIVersion is this trellis-cli build's version. Updates for it are
	// reported by the CLI's own update notifier, not by doctor.
	CLIVersion string
	// TrellisVersion is the project's trellis/VERSION ("" if missing).
	TrellisVersion string
	Requirements   []trellis.Requirement
	// CacheDir holds the cached latest Trellis release (checked at most
	// once a day).
	CacheDir string
	Client   *http.Client
	// Offline skips the GitHub release lookup.
	Offline bool
}

// LocalChecks checks the local machine and project: requirements and
// whether the project's Trellis is on the latest release.
func LocalChecks(o LocalOptions) []Result {
	var results []Result

	for _, req := range o.Requirements {
		results = append(results, requirementResult(req))
	}

	results = append(results,
		Result{ID: "trellis-cli", Title: "trellis-cli", Status: StatusInfo, Message: o.CLIVersion},
		o.trellisResult(),
	)

	return results
}

func requirementResult(req trellis.Requirement) Result {
	r := Result{ID: "requirement-" + strings.ToLower(req.Name), Title: req.Name}

	res, err := req.Check()
	switch {
	case err != nil:
		r.Status = StatusUnknown
		r.Message = fmt.Sprintf("couldn't check %s: %v", req.Name, err)
	case !res.Installed && req.Optional:
		r.Status = StatusSkip
		r.Message = "not installed (optional)"
	case !res.Installed:
		r.Status = StatusFail
		r.Message = fmt.Sprintf("not installed (requires %s)", req.VersionConstraint)
		r.Details = []string{req.Url}
	case !res.Satisfied:
		r.Status = StatusFail
		if req.Optional {
			r.Status = StatusWarn
		}
		r.Message = fmt.Sprintf("%s (requires %s)", res.Version, req.VersionConstraint)
		r.Details = []string{req.Url}
	default:
		r.Status = StatusOK
		r.Message = res.Version
	}

	return r
}

func (o LocalOptions) trellisResult() Result {
	r := Result{ID: "trellis-version", Title: "Trellis"}
	current := o.TrellisVersion

	if current == "" {
		r.Status = StatusSkip
		r.Message = "version unknown (no VERSION file)"
		r.Details = []string{"https://github.com/roots/trellis/releases"}
		return r
	}

	if _, err := version.NewVersion(current); err != nil {
		r.Status = StatusSkip
		r.Message = current + " (not a release)"
		return r
	}

	if o.Offline {
		r.Status = StatusInfo
		r.Message = current
		return r
	}

	// Cached separately from the CLI's own update notifier.
	release, err := o.latestRelease(trellisRepo, current, "trellis")
	switch {
	case err != nil:
		// Being behind is at worst a warning, so a failed lookup isn't
		// treated as unknown.
		r.Status = StatusInfo
		r.Message = current + " (couldn't check for the latest release)"
		r.Details = []string{githubErrorDetail(err)}
	case release != nil:
		r.Status = StatusWarn
		r.Message = fmt.Sprintf("%s → %s available", current, release.Version)
		r.Details = []string{fmt.Sprintf("https://github.com/%s/compare/%s...%s", trellisRepo, current, release.Version)}
	default:
		r.Status = StatusOK
		r.Message = current + " (latest)"
	}

	return r
}

// latestRelease returns the latest GitHub release if it's newer than
// current, using the update notifier's 24 hour cache.
func (o LocalOptions) latestRelease(repo, current, cacheSubdir string) (*github.Release, error) {
	if o.CacheDir == "" {
		return nil, fmt.Errorf("no cache directory")
	}

	client := o.Client
	if client == nil {
		client = github.Client
	}

	notifier := &update.Notifier{
		CacheDir:   filepath.Join(o.CacheDir, cacheSubdir),
		ForceCheck: true,
		Repo:       repo,
		Version:    current,
		Client:     client,
	}

	return notifier.CheckForUpdate()
}

// githubErrorDetail summarizes a GitHub API error. Error bodies can be long
// JSON (and a rate limit message includes the caller's IP address).
func githubErrorDetail(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "rate limit") {
		return "GitHub API rate limit exceeded; try again later or use --offline"
	}
	if body := strings.TrimSpace(msg); strings.HasPrefix(body, "{") || strings.HasPrefix(body, "<") {
		return "GitHub API request failed"
	}
	if line := firstLine(msg, 200); line != "" {
		return line
	}
	return "GitHub API request failed"
}

// firstLine returns the first line of s, truncated to max bytes.
func firstLine(s string, max int) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
