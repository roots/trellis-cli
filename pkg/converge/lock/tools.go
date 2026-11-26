package lock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ToolNames are the binaries Trellis downloads onto servers.
var ToolNames = []string{"composer", "wp-cli"}

// ToolURL is where version of tool is downloaded from. Version may be a
// moving channel (composer's latest-2.x) until it is locked.
func ToolURL(name, version string) string {
	switch name {
	case "composer":
		return "https://getcomposer.org/download/" + version + "/composer.phar"
	case "wp-cli":
		return fmt.Sprintf("https://github.com/wp-cli/wp-cli/releases/download/v%s/wp-cli-%s.phar", version, version)
	}
	return ""
}

// ToolResolver finds a tool's current release and its checksum.
type ToolResolver interface {
	Resolve(ctx context.Context, name string) (Tool, error)
}

// HTTPToolResolver resolves tools from their upstreams: the latest Composer
// 2.x (getcomposer.org/versions) and the latest wp-cli release (GitHub).
// It downloads each binary, hashes it, and checks the hash against the
// upstream's published checksum.
type HTTPToolResolver struct {
	Client *http.Client
}

func (r HTTPToolResolver) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (r HTTPToolResolver) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 128<<20))
}

func (r HTTPToolResolver) Resolve(ctx context.Context, name string) (Tool, error) {
	var version, sumURL string
	switch name {
	case "composer":
		b, err := r.get(ctx, "https://getcomposer.org/versions")
		if err != nil {
			return Tool{}, err
		}
		var v struct {
			Stable []struct{ Version string } `json:"stable"`
		}
		if err := json.Unmarshal(b, &v); err != nil {
			return Tool{}, fmt.Errorf("getcomposer.org/versions: %w", err)
		}
		for _, s := range v.Stable {
			if strings.HasPrefix(s.Version, "2.") {
				version = s.Version
				break
			}
		}
		if version == "" {
			return Tool{}, fmt.Errorf("no stable Composer 2.x release listed")
		}
		sumURL = ToolURL(name, version) + ".sha256sum"
	case "wp-cli":
		b, err := r.get(ctx, "https://api.github.com/repos/wp-cli/wp-cli/releases/latest")
		if err != nil {
			return Tool{}, err
		}
		var rel struct {
			Tag string `json:"tag_name"`
		}
		if err := json.Unmarshal(b, &rel); err != nil || rel.Tag == "" {
			return Tool{}, fmt.Errorf("wp-cli latest release: %v", err)
		}
		version = strings.TrimPrefix(rel.Tag, "v")
		sumURL = ToolURL(name, version) + ".sha256"
	default:
		return Tool{}, fmt.Errorf("unknown tool %q", name)
	}
	bin, err := r.get(ctx, ToolURL(name, version))
	if err != nil {
		return Tool{}, err
	}
	sum := sha256.Sum256(bin)
	got := hex.EncodeToString(sum[:])
	published, err := r.get(ctx, sumURL)
	if err != nil {
		return Tool{}, fmt.Errorf("%s %s checksum: %w", name, version, err)
	}
	if f := strings.Fields(string(published)); len(f) == 0 || !strings.EqualFold(f[0], got) {
		return Tool{}, fmt.Errorf("%s %s: downloaded sha256 %s doesn't match the published checksum", name, version, got)
	}
	return Tool{Version: version, SHA256: got}, nil
}

// updateTools pins every tool: existing pins are kept unless refresh.
func updateTools(ctx context.Context, l, out *Lock, r ToolResolver, refresh bool) error {
	out.Tools = map[string]Tool{}
	for _, name := range ToolNames {
		if t, ok := l.Tools[name]; ok && t.SHA256 != "" && !refresh {
			out.Tools[name] = t
			continue
		}
		t, err := r.Resolve(ctx, name)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", name, err)
		}
		out.Tools[name] = t
	}
	return nil
}
