package doctor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/github"
)

func TestTrellisRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/roots/trellis/releases/latest":
			_, _ = w.Write([]byte(`{"tag_name":"v1.33.0","html_url":"https://github.com/roots/trellis/releases/tag/v1.33.0"}`))
		case "/rate-limited/repos/roots/trellis/releases/latest":
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 203.0.113.5."}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	old := github.BaseURL
	defer func() { github.BaseURL = old }()

	cases := []struct {
		name     string
		baseURL  string
		current  string
		offline  bool
		status   Status
		contains string
		excludes string
	}{
		{"behind", "", "v1.31.0", false, StatusWarn, "compare/v1.31.0...v1.33.0", ""},
		{"latest", "", "v1.33.0", false, StatusOK, "(latest)", ""},
		{"dev", "", "dev", false, StatusSkip, "not a release", ""},
		{"no VERSION", "", "", false, StatusSkip, "no VERSION file", ""},
		{"offline", "", "v1.31.0", true, StatusInfo, "v1.31.0", ""},
		{"not found", "/missing", "v1.31.0", false, StatusInfo, "GitHub API request failed", ""},
		{"rate limited", "/rate-limited", "v1.31.0", false, StatusInfo, "rate limit exceeded", "203.0.113.5"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			github.BaseURL = server.URL + tc.baseURL

			r := TrellisRelease(tc.current, t.TempDir(), tc.offline)
			text := r.Message + " " + strings.Join(r.Details, " ")

			if r.Status != tc.status {
				t.Errorf("expected %s, got %s: %s", tc.status, r.Status, text)
			}
			if !strings.Contains(text, tc.contains) {
				t.Errorf("expected %q in %q", tc.contains, text)
			}
			if tc.excludes != "" && strings.Contains(text, tc.excludes) {
				t.Errorf("didn't expect %q in %q", tc.excludes, text)
			}
		})
	}
}
