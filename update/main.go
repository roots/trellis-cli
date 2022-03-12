package update

import (
	"github.com/roots/trellis-cli/github"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/mattn/go-isatty"
	"gopkg.in/yaml.v2"
)

type Notifier struct {
	CacheDir   string
	ForceCheck bool
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

	if n.Repo == "" || n.CacheDir == "" {
		return false
	}

	// skip completion commands
	if os.Getenv("COMP_LINE") != "" {
		return false
	}

	if os.Getenv("TRELLIS_NO_UPDATE_NOTIFIER") != "" {
		return false
	}

	// skip on common CI environments
	if os.Getenv("CI") != "" {
		return false
	}

	// skip on non-terminals
	fd := os.Stdout.Fd()
	if !(isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)) {
		return false
	}

	return true
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

	err = setStateEntry(stateFilePath, time.Now(), latestRelease)
	if err != nil {
		return nil, err
	}

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
