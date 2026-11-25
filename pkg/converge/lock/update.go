package lock

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"
)

// KeyFetcher resolves a GitHub username to its public SSH keys.
type KeyFetcher interface {
	GitHubKeys(ctx context.Context, user string) ([]string, error)
}

// UpdateOptions controls Update.
type UpdateOptions struct {
	// Refresh re-resolves entries that are already locked.
	Refresh bool
}

// Update resolves every network-dependent reference in refs and returns a
// new lock. Existing entries are kept unless opts.Refresh is set; entries
// no longer referenced are dropped so the lock never grows stale.
func Update(ctx context.Context, l *Lock, keyRefs []string, f KeyFetcher, opts UpdateOptions) (*Lock, error) {
	out := *l
	out.SSHKeys = map[string][]string{}
	for _, ref := range dedupe(keyRefs) {
		user, ok := strings.CutPrefix(ref, "github:")
		if !ok {
			continue
		}
		if keys, ok := l.SSHKeys[ref]; ok && !opts.Refresh {
			out.SSHKeys[ref] = keys
			continue
		}
		keys, err := f.GitHubKeys(ctx, user)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", ref, err)
		}
		if len(keys) == 0 {
			return nil, fmt.Errorf("resolve %s: user has no public SSH keys", ref)
		}
		out.SSHKeys[ref] = keys
	}
	return &out, nil
}

func dedupe(s []string) []string {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return slices.Sorted(maps.Keys(m))
}

// HTTPKeyFetcher fetches https://github.com/<user>.keys.
type HTTPKeyFetcher struct {
	Client  *http.Client
	BaseURL string // default https://github.com
}

func (h HTTPKeyFetcher) GitHubKeys(ctx context.Context, user string) ([]string, error) {
	base := h.BaseURL
	if base == "" {
		base = "https://github.com"
	}
	client := h.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+user+".keys", nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", req.URL, resp.Status)
	}
	var keys []string
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 1<<20))
	for sc.Scan() {
		if line := strings.TrimSpace(sc.Text()); line != "" {
			keys = append(keys, line)
		}
	}
	return keys, sc.Err()
}
