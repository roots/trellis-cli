package lock

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
