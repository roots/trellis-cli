package lock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type fakeFetcher map[string][]string

func (f fakeFetcher) GitHubKeys(_ context.Context, user string) ([]string, error) {
	k, ok := f[user]
	if !ok {
		return nil, fmt.Errorf("no such user")
	}
	return k, nil
}

func TestRoundTrip(t *testing.T) {
	l := New()
	l.Packages = map[string]string{"nginx": "1.29.3", "a": "1"}
	l.SSHKeys = map[string][]string{"github:x": {"ssh-ed25519 AAA"}}
	b, err := l.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	got, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff(l, got); diff != "" {
		t.Error(diff)
	}
	b2, _ := got.Marshal()
	if string(b) != string(b2) {
		t.Error("marshal is not deterministic")
	}
}

func TestUpdateKeepsPrunesAndResolves(t *testing.T) {
	l := New()
	l.SSHKeys = map[string][]string{
		"github:kept":  {"old-key"},
		"github:stale": {"gone"},
	}
	f := fakeFetcher{"kept": {"new-key"}, "new": {"k1", "k2"}}
	refs := []string{"github:kept", "github:new", "file:keys/a.pub", "ssh-ed25519 literal", "github:new"}

	got, err := Update(context.Background(), l, refs, f, UpdateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"github:kept": {"old-key"}, "github:new": {"k1", "k2"}}
	if diff := cmp.Diff(want, got.SSHKeys); diff != "" {
		t.Error(diff)
	}

	got, err = Update(context.Background(), l, refs, f, UpdateOptions{Refresh: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.SSHKeys["github:kept"][0] != "new-key" {
		t.Error("refresh did not re-resolve")
	}
}

func TestHTTPKeyFetcher(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/someone.keys" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, "ssh-ed25519 AAA one\n\nssh-rsa BBB two\n")
	}))
	defer srv.Close()

	keys, err := HTTPKeyFetcher{BaseURL: srv.URL}.GitHubKeys(context.Background(), "someone")
	if err != nil {
		t.Fatal(err)
	}
	if diff := cmp.Diff([]string{"ssh-ed25519 AAA one", "ssh-rsa BBB two"}, keys); diff != "" {
		t.Error(diff)
	}
	if _, err := (HTTPKeyFetcher{BaseURL: srv.URL}).GitHubKeys(context.Background(), "nobody"); err == nil {
		t.Error("expected error for 404")
	}
}

type fakeTools map[string]Tool

func (f fakeTools) Resolve(_ context.Context, name string) (Tool, error) { return f[name], nil }

func TestUpdatePinsTools(t *testing.T) {
	l := New()
	l.Tools = map[string]Tool{"wp-cli": {Version: "2.11.0", SHA256: "old"}}
	r := fakeTools{"wp-cli": {Version: "2.12.0", SHA256: "new"}, "composer": {Version: "2.11.3", SHA256: "c"}}
	got, err := Update(context.Background(), l, nil, fakeFetcher{}, UpdateOptions{Tools: r})
	if err != nil {
		t.Fatal(err)
	}
	if got.Tools["wp-cli"].SHA256 != "old" || got.Tools["composer"].Version != "2.11.3" {
		t.Errorf("existing pins must be kept, missing ones resolved: %+v", got.Tools)
	}
	got, _ = Update(context.Background(), l, nil, fakeFetcher{}, UpdateOptions{Tools: r, Refresh: true})
	if got.Tools["wp-cli"].Version != "2.12.0" {
		t.Errorf("--refresh must re-resolve: %+v", got.Tools)
	}
}

func TestHTTPToolResolverChecksChecksums(t *testing.T) {
	bin := []byte("composer phar")
	sum := sha256.Sum256(bin)
	published := hex.EncodeToString(sum[:])
	mux := http.NewServeMux()
	mux.HandleFunc("/versions", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"stable":[{"version":"2.11.3"},{"version":"2.2.33"}]}`))
	})
	mux.HandleFunc("/download/2.11.3/composer.phar", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(bin) })
	mux.HandleFunc("/download/2.11.3/composer.phar.sha256sum", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(published + "  composer.phar\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	r := HTTPToolResolver{Client: rewrite(srv.URL)}
	tool, err := r.Resolve(context.Background(), "composer")
	if err != nil || tool.Version != "2.11.3" || tool.SHA256 != published {
		t.Fatalf("resolve = %+v, %v", tool, err)
	}

	published = strings.Repeat("0", 64)
	if _, err := r.Resolve(context.Background(), "composer"); err == nil || !strings.Contains(err.Error(), "doesn't match") {
		t.Errorf("a checksum mismatch must fail, got %v", err)
	}
}

// rewrite sends every request to base (a test server), keeping the path.
func rewrite(base string) *http.Client {
	return &http.Client{Transport: rewriteTransport(base)}
}

type rewriteTransport string

func (b rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := url.Parse(string(b))
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = u.Scheme, u.Host
	return http.DefaultTransport.RoundTrip(req)
}
