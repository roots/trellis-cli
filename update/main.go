package update

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/mattn/go-isatty"
	"github.com/roots/trellis-cli/github"
	"gopkg.in/yaml.v2"
)

type Notifier struct {
	CacheDir   string
	ForceCheck bool
	SkipCheck  bool
	Repo       string
	Version    string
	Client     *http.Client
}

type StateEntry struct {
	CheckedForUpdateAt time.Time       `yaml:"checked_for_update_at"`
	LatestRelease      *github.Release `yaml:"latest_release"`
}

func (n *Notifier) CheckForUpdate() (*github.Release, error) {
	if !n.shouldCheckForUpdate() {
		return nil, nil
	}

	stateFilePath := filepath.Join(n.CacheDir, "state.yml")
	latestRelease, err := n.getLatestReleaseInfo(stateFilePath)
	if err != nil {
		return nil, err
	}

	if versionGreaterThan(latestRelease.Version, n.Version) {
		return latestRelease, nil
	}

	return nil, nil
}

func (n *Notifier) shouldCheckForUpdate() bool {
	if n.ForceCheck {
		return true
	}

	if n.SkipCheck {
		return false
	}

	if n.Repo == "" || n.CacheDir == "" {
		return false
	}

	// skip completion commands
	if os.Getenv("COMP_LINE") != "" {
		return false
	}

	// skip on common CI environments
	if os.Getenv("CI") != "" {
		return false
	}

	// skip on non-terminals; the notice goes to stderr after the command's
	// output on stdout
	return isTerminal(os.Stdout) && isTerminal(os.Stderr)
}

func isTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
}

func (n *Notifier) getLatestReleaseInfo(stateFilePath string) (*github.Release, error) {
	stateEntry, err := getStateEntry(stateFilePath)
	if err == nil && time.Since(stateEntry.CheckedForUpdateAt).Hours() < 24 {
		return stateEntry.LatestRelease, nil
	}

	latestRelease, err := github.FetchLatestRelease(n.Repo, n.Client)
	if err != nil {
		return nil, err
	}

	// Failing to cache the result (e.g. a read-only home directory) only
	// means checking again next time.
	_ = setStateEntry(stateFilePath, time.Now(), latestRelease)

	return latestRelease, nil
}

func getStateEntry(stateFilePath string) (*StateEntry, error) {
	content, err := os.ReadFile(stateFilePath)
	if err != nil {
		return nil, err
	}

	var stateEntry StateEntry
	err = yaml.Unmarshal(content, &stateEntry)
	if err != nil {
		return nil, err
	}

	return &stateEntry, nil
}

func setStateEntry(stateFilePath string, t time.Time, r *github.Release) error {
	data := StateEntry{CheckedForUpdateAt: t, LatestRelease: r}
	content, err := yaml.Marshal(data)
	if err != nil {
		return err
	}

	if err = os.MkdirAll(filepath.Dir(stateFilePath), 0771); err != nil {
		return err
	}

	_ = os.WriteFile(stateFilePath, content, 0600)

	return nil
}

func versionGreaterThan(v, w string) bool {
	vv, ve := version.NewVersion(v)
	vw, we := version.NewVersion(w)

	return ve == nil && we == nil && vv.GreaterThan(vw)
}
