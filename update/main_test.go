package update

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/roots/trellis-cli/github"
)

type Env struct {
	key   string
	value string
}

func TestDoesNotCheckForUpdate(t *testing.T) {
	cacheDir := t.TempDir()

	cases := []struct {
		name          string
		repo          string
		cacheDir      string
		env           []Env
		latestRelease *github.Release
	}{
		{
			"no_repo",
			"",
			cacheDir,
			nil,
			nil,
		},
		{
			"no_cache_dir",
			"roots/trellis-cli",
			"",
			nil,
			nil,
		},
		{
			"completion_command",
			"roots/trellis-cli",
			cacheDir,
			[]Env{{"COMP_LINE", "foo"}},
			nil,
		},
		{
			"TRELLIS_NO_UPDATE_NOTIFIER set",
			"roots/trellis-cli",
			cacheDir,
			[]Env{{"TRELLIS_NO_UPDATE_NOTIFIER", "1"}},
			nil,
		},
		{
			"CI set",
			"roots/trellis-cli",
			cacheDir,
			[]Env{{"CI", "1"}},
			nil,
		},
	}

	for _, tc := range cases {
		updateNotifier := &Notifier{CacheDir: tc.cacheDir, Repo: tc.repo, Version: "1.0"}
		release, err := updateNotifier.CheckForUpdate()

		if err != nil {
			t.Errorf("expected no error, but got %q", err)
		}

		if release != tc.latestRelease {
			t.Errorf("expected release %s but got %s", tc.latestRelease, release)
		}
	}
}

func TestCheckForUpdate(t *testing.T) {
	now := time.Now()
	client := http.DefaultClient

	cases := []struct {
		name           string
		version        string
		stateEntry     string
		githubResponse string
		latestRelease  *github.Release
	}{
		{
			"state_cache_newer_version",
			"v1.0",
			fmt.Sprintf(`
checked_for_update_at: %s
latest_release:
  version: v1.1
  zipurl: https://api.github.com/repos/roots/trellis-cli/zipball/v1.1
  url: https://github.com/roots/trellis-cli/releases/tag/v1.1
`, now.Format(time.RFC3339)),
			"",
			&github.Release{
				Version: "v1.1",
				URL:     "https://github.com/roots/trellis-cli/releases/tag/v1.1",
				ZipUrl:  "https://api.github.com/repos/roots/trellis-cli/zipball/v1.1",
			},
		},
		{
			"state_cache_older_version",
			"v1.0",
			fmt.Sprintf(`
checked_for_update_at: %s
latest_release:
  version: v0.9
  zipurl: https://api.github.com/repos/roots/trellis-cli/zipball/v0.9
  url: https://github.com/roots/trellis-cli/releases/tag/v0.9
`, now.Format(time.RFC3339)),
			"",
			nil,
		},
		{
			"state_cache_same_version",
			"v1.1",
			fmt.Sprintf(`
checked_for_update_at: %s
latest_release:
  version: v1.1
  zipurl: https://api.github.com/repos/roots/trellis-cli/zipball/v1.0
  url: https://github.com/roots/trellis-cli/releases/tag/v1.0
`, now.Format(time.RFC3339)),
			"",
			nil,
		},
		{
			"state_cache_newer_version_older_than_24_hours",
			"v1.0",
			fmt.Sprintf(`
checked_for_update_at: %s
latest_release:
  version: v1.0
  zipurl: https://api.github.com/repos/roots/trellis-cli/zipball/v1.0
  url: https://github.com/roots/trellis-cli/releases/tag/v1.0
`, now.Add(-time.Hour*25).Format(time.RFC3339)),
			fmt.Sprintf(`{
  "tag_name": "v1.1",
  "html_url": "https://github.com/roots/trellis-cli/releases/tag/v1.1",
  "zipball_url": "https://api.github.com/repos/roots/trellis-cli/zipball/v1.1"
}`),
			&github.Release{
				Version: "v1.1",
				URL:     "https://github.com/roots/trellis-cli/releases/tag/v1.1",
				ZipUrl:  "https://api.github.com/repos/roots/trellis-cli/zipball/v1.1",
			},
		},
		{
			"state_cache_sameversion_older_than_24_hours",
			"v1.0",
			fmt.Sprintf(`
checked_for_update_at: %s
latest_release:
  version: v1.0
  zipurl: https://api.github.com/repos/roots/trellis-cli/zipball/v1.0
  url: https://github.com/roots/trellis-cli/releases/tag/v1.0
`, now.Add(-time.Hour*25).Format(time.RFC3339)),
			fmt.Sprintf(`{
  "tag_name": "v1.0",
  "html_url": "https://github.com/roots/trellis-cli/releases/tag/v1.0",
  "zipball_url": "https://api.github.com/repos/roots/trellis-cli/zipball/v1.0"
}`),
			nil,
		},
		{
			"no_cache_newer_version",
			"v1.0",
			"",
			fmt.Sprintf(`{
  "tag_name": "v1.1",
  "html_url": "https://github.com/roots/trellis-cli/releases/tag/v1.1",
  "zipball_url": "https://api.github.com/repos/roots/trellis-cli/zipball/v1.1"
}`),
			&github.Release{
				Version: "v1.1",
				URL:     "https://github.com/roots/trellis-cli/releases/tag/v1.1",
				ZipUrl:  "https://api.github.com/repos/roots/trellis-cli/zipball/v1.1",
			},
		},
		{
			"no_cache_same_version",
			"v1.0",
			"",
			fmt.Sprintf(`{
  "tag_name": "v1.0",
  "html_url": "https://github.com/roots/trellis-cli/releases/tag/v1.0",
  "zipball_url": "https://api.github.com/repos/roots/trellis-cli/zipball/v1.0"
}`),
			nil,
		},
		{
			"no_cache_older_version",
			"v1.1",
			"",
			fmt.Sprintf(`{
  "tag_name": "v1.0",
  "html_url": "https://github.com/roots/trellis-cli/releases/tag/v1.0",
  "zipball_url": "https://api.github.com/repos/roots/trellis-cli/zipball/v1.0"
}`),
			nil,
		},
	}

	for _, tc := range cases {
		cacheDir := t.TempDir()

		if tc.stateEntry != "" {
			_ = os.WriteFile(filepath.Join(cacheDir, "state.yml"), []byte(tc.stateEntry), 0600)
		}

		if tc.githubResponse != "" {
			server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
				rw.Write([]byte(tc.githubResponse))
			}))
			defer server.Close()

			github.BaseURL = server.URL
			client = server.Client()
		}

		updateNotifier := &Notifier{
			ForceCheck: true,
			CacheDir:   cacheDir,
			Repo:       "roots/trellis-cli",
			Version:    tc.version,
			Client:     client,
		}

		release, _ := updateNotifier.CheckForUpdate()
		os.RemoveAll(cacheDir)

		if !cmp.Equal(tc.latestRelease, release) {
			t.Errorf("expected release %s but got %s", tc.latestRelease, release)
		}
	}
}
