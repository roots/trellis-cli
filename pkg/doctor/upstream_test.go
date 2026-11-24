package doctor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func phpNetServer(t *testing.T, responses map[string]string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if _, ok := r.URL.Query()["json"]; !ok {
			t.Errorf("expected json query param, got %s", r.URL.RawQuery)
		}
		version, hasVersion := r.URL.Query()["version"]
		body, ok := "", false
		if hasVersion {
			body, ok = responses[version[0]]
		} else {
			body, ok = responses["*"]
		}
		if !ok {
			body = `{"error":"Unknown version"}`
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	old := PHPReleasesURL
	PHPReleasesURL = server.URL
	t.Cleanup(func() { PHPReleasesURL = old })

	return server, &calls
}

func TestPHPNetRelease(t *testing.T) {
	server, calls := phpNetServer(t, map[string]string{
		"8.3": `{"announcement":true,"tags":["security"],"date":"19 Nov 2026","source":[],"supported_versions":["8.2","8.3","8.4","8.5"],"version":"8.3.37"}`,
		"8.1": `{"announcement":{"English":"/releases/8_1_34.php"},"tags":["security"],"date":"18 Dec 2025","museum":false,"supported_versions":["8.2","8.3"],"version":"8.1.34"}`,
		"7.4": `{"announcement":{"English":"x"},"tags":[],"date":"28 Nov 2022","supported_versions":[],"version":"7.4.33"}`,
		"8.4": `{"tags":[],"date":"3 Dec 2026","supported_versions":["8.4"],"version":"8.4.21"}`,
		"*":   `{"8":{"version":"8.6.0","supported_versions":["8.2","8.3","8.4","8.5","8.6"]},"7":{"version":"7.4.33","supported_versions":[]}}`,
	})

	p := &PHPNet{Client: server.Client()}

	rel := p.Release("8.3")
	if rel == nil {
		t.Fatal("expected release for 8.3")
	}
	if rel.Version != "8.3.37" || !rel.Security || rel.Supported == nil || !*rel.Supported {
		t.Errorf("unexpected 8.3 release: %+v", rel)
	}
	if want := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC); !rel.Date.Equal(want) {
		t.Errorf("unexpected date %v", rel.Date)
	}

	if rel := p.Release("8.1"); rel == nil || rel.Supported == nil || *rel.Supported {
		t.Errorf("expected unsupported 8.1, got %+v", rel)
	}
	if rel := p.Release("7.4"); rel == nil || rel.Supported == nil || *rel.Supported {
		t.Errorf("expected unsupported 7.4 from the all-versions list, got %+v", rel)
	}
	if rel := p.Release("8.4"); rel == nil || rel.Security || rel.Date.Day() != 3 {
		t.Errorf("unexpected 8.4 release: %+v", rel)
	}
	if rel := p.Release("9.9"); rel != nil {
		t.Errorf("expected nil for unknown branch, got %+v", rel)
	}
	if rel := p.Release(""); rel != nil {
		t.Errorf("expected nil for empty branch, got %+v", rel)
	}

	before := calls.Load()
	p.Release("8.3")
	p.Release("9.9")
	if calls.Load() != before {
		t.Error("expected memoized lookups not to refetch")
	}

	if err := p.Err(); err != nil {
		t.Errorf("an unknown branch isn't a lookup failure, got %v", err)
	}
}

func TestPHPNetReleaseRejectsMismatchedBranch(t *testing.T) {
	server, _ := phpNetServer(t, map[string]string{
		"8.3": `{"tags":[],"date":"19 Nov 2026","version":"8.4.21"}`,
	})

	p := &PHPNet{Client: server.Client()}
	if rel := p.Release("8.3"); rel != nil {
		t.Errorf("expected nil, got %+v", rel)
	}
	if p.Err() == nil {
		t.Error("expected a lookup failure")
	}
}

func TestPHPNetReleaseBadDate(t *testing.T) {
	server, _ := phpNetServer(t, map[string]string{
		"8.3": `{"tags":[],"date":"2026-11-19","version":"8.3.37"}`,
	})

	p := &PHPNet{Client: server.Client()}
	if rel := p.Release("8.3"); rel != nil {
		t.Errorf("expected nil, got %+v", rel)
	}
	if p.Err() == nil {
		t.Error("expected a lookup failure")
	}
}

func TestPHPNetReleaseServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadGateway)
	}))
	defer server.Close()

	old := PHPReleasesURL
	PHPReleasesURL = server.URL
	defer func() { PHPReleasesURL = old }()

	p := &PHPNet{Client: server.Client()}
	if rel := p.Release("8.3"); rel != nil {
		t.Errorf("expected nil, got %+v", rel)
	}
	if err := p.Err(); err == nil || !strings.Contains(err.Error(), "502") {
		t.Errorf("expected a 502 lookup failure, got %v", err)
	}
}

func TestPHPNetReleaseSupportUnknown(t *testing.T) {
	// No site-wide list available: support stays unknown.
	server, _ := phpNetServer(t, map[string]string{
		"7.4": `{"tags":[],"date":"28 Nov 2022","supported_versions":[],"version":"7.4.33"}`,
	})

	p := &PHPNet{Client: server.Client()}
	if rel := p.Release("7.4"); rel == nil || rel.Supported != nil {
		t.Errorf("expected unknown support, got %+v", rel)
	}
	if p.Err() == nil {
		t.Error("expected the failed supported versions lookup to be recorded")
	}
}
