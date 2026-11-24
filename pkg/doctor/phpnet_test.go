package doctor

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// phpNetServer serves php.net's releases API with a body per branch; status
// 0 means 200.
func phpNetServer(t *testing.T, status int, responses map[string]string) (*phpNetClient, *int) {
	t.Helper()
	calls := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if _, ok := r.URL.Query()["json"]; !ok {
			t.Errorf("expected json query param, got %s", r.URL.RawQuery)
		}
		body, ok := responses[r.URL.Query().Get("version")]
		if !ok {
			body = `{"error":"Unknown version"}`
		}
		if status != 0 {
			w.WriteHeader(status)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return &phpNetClient{url: server.URL, client: server.Client()}, &calls
}

func TestPHPNetLatest(t *testing.T) {
	p, calls := phpNetServer(t, 0, map[string]string{
		"8.3": `{"announcement":true,"tags":["security"],"date":"19 Nov 2026","source":[],"supported_versions":["8.2","8.3","8.4","8.5"],"version":"8.3.37"}`,
		"8.1": `{"announcement":{"English":"/releases/8_1_34.php"},"tags":["security"],"date":"18 Dec 2025","museum":false,"supported_versions":["8.2","8.3"],"version":"8.1.34"}`,
		"7.4": `{"announcement":{"English":"x"},"tags":[],"date":"28 Nov 2022","supported_versions":[],"version":"7.4.33"}`,
		"8.4": `{"tags":[],"date":"3 Dec 2026","supported_versions":["8.4"],"version":"8.4.21"}`,
	})

	rel, err := p.latest("8.3")
	if err != nil || rel == nil || rel.Version != "8.3.37" || !rel.Security || rel.Supported == nil || !*rel.Supported {
		t.Fatalf("unexpected 8.3 release: %+v", rel)
	}
	if want := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC); !rel.Date.Equal(want) {
		t.Errorf("unexpected date %v", rel.Date)
	}

	if rel, _ := p.latest("8.1"); rel == nil || rel.Supported == nil || *rel.Supported {
		t.Errorf("expected unsupported 8.1, got %+v", rel)
	}
	if rel, _ := p.latest("7.4"); rel == nil || rel.Supported != nil {
		t.Errorf("expected unknown support for 7.4, got %+v", rel)
	}
	if rel, _ := p.latest("8.4"); rel == nil || rel.Security || rel.Date.Day() != 3 {
		t.Errorf("unexpected 8.4 release: %+v", rel)
	}
	if rel, err := p.latest("9.9"); rel != nil || err != nil {
		t.Errorf("expected nil without an error for an unknown branch, got %+v, %v", rel, err)
	}

	before := *calls
	_, _ = p.latest("8.3")
	_, _ = p.latest("9.9")
	if *calls != before {
		t.Error("expected memoized lookups not to refetch")
	}
}

func TestPHPNetLatestFailures(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
	}{
		"mismatched branch": {0, `{"tags":[],"date":"19 Nov 2026","version":"8.4.21"}`},
		"bad date":          {0, `{"tags":[],"date":"2026-11-19","version":"8.3.37"}`},
		"server error":      {http.StatusBadGateway, `nope`},
		"invalid json":      {0, `<html>`},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			p, _ := phpNetServer(t, tc.status, map[string]string{"8.3": tc.body})

			if rel, err := p.latest("8.3"); rel != nil || err == nil {
				t.Errorf("expected a lookup failure, got %+v, %v", rel, err)
			}
		})
	}
}
