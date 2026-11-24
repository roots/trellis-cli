package doctor

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/github"
	"github.com/roots/trellis-cli/trellis"
)

func githubServer(t *testing.T, latest map[string]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for repo, tag := range latest {
			if r.URL.Path == "/repos/"+repo+"/releases/latest" {
				_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","html_url":"https://github.com/` + repo + `/releases/tag/` + tag + `"}`))
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	old := github.BaseURL
	github.BaseURL = server.URL
	t.Cleanup(func() { github.BaseURL = old })

	return server
}

func localResult(t *testing.T, results []Result, id string) Result {
	t.Helper()
	for _, r := range results {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("no %s result in %+v", id, results)
	return Result{}
}

func TestLocalChecksVersions(t *testing.T) {
	server := githubServer(t, map[string]string{"roots/trellis": "v1.33.0"})

	cases := []struct {
		name       string
		opts       LocalOptions
		id         string
		status     Status
		contains   string
		noCacheDir bool
	}{
		{"trellis behind", LocalOptions{TrellisVersion: "v1.31.0"}, "trellis-version", StatusWarn, "compare/v1.31.0...v1.33.0", false},
		{"trellis latest", LocalOptions{TrellisVersion: "v1.33.0"}, "trellis-version", StatusOK, "(latest)", false},
		{"trellis dev", LocalOptions{TrellisVersion: "dev"}, "trellis-version", StatusSkip, "not a release", false},
		{"trellis no VERSION", LocalOptions{}, "trellis-version", StatusSkip, "no VERSION file", false},
		{"trellis offline", LocalOptions{TrellisVersion: "v1.31.0", Offline: true}, "trellis-version", StatusInfo, "v1.31.0", false},
		{"trellis no cache dir", LocalOptions{TrellisVersion: "v1.31.0"}, "trellis-version", StatusInfo, "couldn't check", true},
		{"trellis release not found", LocalOptions{TrellisVersion: "v1.31.0"}, "trellis-version", StatusInfo, "couldn't check", false},
		{"cli version", LocalOptions{CLIVersion: "1.20.0"}, "trellis-cli", StatusInfo, "1.20.0", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.opts.Client = server.Client()
			if tc.name == "trellis release not found" {
				old := github.BaseURL
				github.BaseURL = server.URL + "/missing"
				defer func() { github.BaseURL = old }()
			}
			if !tc.noCacheDir {
				tc.opts.CacheDir = t.TempDir()
			}

			r := localResult(t, LocalChecks(tc.opts), tc.id)

			if r.Status != tc.status {
				t.Errorf("expected %s, got %s: %s", tc.status, r.Status, r.Message)
			}
			if text := r.Message + " " + strings.Join(r.Details, " "); !strings.Contains(text, tc.contains) {
				t.Errorf("expected %q in %q", tc.contains, text)
			}
		})
	}
}

func TestLocalChecksRequirements(t *testing.T) {
	missing := func(optional bool) trellis.Requirement {
		return trellis.Requirement{
			Name:              "Missing",
			Command:           "trellis-doctor-test-missing-command",
			Optional:          optional,
			VersionConstraint: ">= 1.0",
			Url:               "https://example.com",
		}
	}

	results := LocalChecks(LocalOptions{Requirements: []trellis.Requirement{missing(false)}, Offline: true})
	if r := localResult(t, results, "requirement-missing"); r.Status != StatusFail || !strings.Contains(r.Message, ">= 1.0") {
		t.Errorf("expected failing required requirement, got %+v", r)
	}

	results = LocalChecks(LocalOptions{Requirements: []trellis.Requirement{missing(true)}, Offline: true})
	if r := localResult(t, results, "requirement-missing"); r.Status != StatusSkip {
		t.Errorf("expected skipped optional requirement, got %+v", r)
	}
}

func TestGithubErrorDetail(t *testing.T) {
	cases := map[string]string{
		`{"message":"API rate limit exceeded for 203.0.113.5. (But here's the good news...)"}`: "GitHub API rate limit exceeded; try again later or use --offline",
		`{"message":"Not Found"}`:              "GitHub API request failed",
		"<!DOCTYPE html><html>Unicorn!</html>": "GitHub API request failed",
		"":                                     "GitHub API request failed",
		"dial tcp: lookup api.github.com: no such host": "dial tcp: lookup api.github.com: no such host",
	}

	for in, want := range cases {
		if got := githubErrorDetail(errors.New(in)); got != want {
			t.Errorf("githubErrorDetail(%q) = %q, want %q", in, got, want)
		}
	}
}
