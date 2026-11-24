package doctor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"
)

// PHPReleasesURL is php.net's releases API.
var PHPReleasesURL = "https://www.php.net/releases/index.php"

// errUnknownBranch is php.net's answer for a branch it doesn't know; that's
// not a lookup failure.
var errUnknownBranch = errors.New("php.net doesn't know this PHP branch")

// PHPNet looks up the latest PHP releases from php.net. Lookups are memoized,
// so each branch is fetched at most once per run. Safe for concurrent use.
type PHPNet struct {
	Client *http.Client

	mu       sync.Mutex
	releases map[string]*PHPRelease
	err      error
	// supported is every branch php.net supports, fetched at most once.
	supported []string
}

type phpNetResponse struct {
	Error             string   `json:"error"`
	Version           string   `json:"version"`
	Date              string   `json:"date"`
	Tags              []string `json:"tags"`
	SupportedVersions []string `json:"supported_versions"`
}

// Release returns the latest release of a PHP branch (e.g. "8.3"), or nil if
// php.net can't be reached or doesn't know the branch. It satisfies
// PHPReleaseLookup.
func (p *PHPNet) Release(branch string) *PHPRelease {
	if branch == "" {
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if rel, ok := p.releases[branch]; ok {
		return rel
	}

	rel, err := p.fetch(branch)
	if err != nil && !errors.Is(err, errUnknownBranch) {
		p.err = fmt.Errorf("PHP %s: %w", branch, err)
	}

	if p.releases == nil {
		p.releases = map[string]*PHPRelease{}
	}
	p.releases[branch] = rel

	return rel
}

func (p *PHPNet) fetch(branch string) (*PHPRelease, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	u := fmt.Sprintf("%s?json&version=%s", PHPReleasesURL, url.QueryEscape(branch))
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("php.net returned %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}

	var data phpNetResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	// Unknown branches return 200 with {"error": "Unknown version"}.
	if data.Error != "" {
		return nil, fmt.Errorf("%w: %s", errUnknownBranch, data.Error)
	}

	if Branch(data.Version) != branch {
		return nil, fmt.Errorf("php.net returned %q for branch %s", data.Version, branch)
	}

	date, err := parsePHPNetDate(data.Date)
	if err != nil {
		return nil, err
	}

	rel := &PHPRelease{
		Version:  data.Version,
		Date:     date,
		Security: slices.Contains(data.Tags, "security"),
	}

	// supported_versions is php.net's list for the branch's major version,
	// which is empty once a major version is entirely unsupported (7.x).
	// Fall back to the list across all major versions.
	supportedVersions := data.SupportedVersions
	if len(supportedVersions) == 0 {
		supportedVersions = p.allSupportedVersions(client)
	}
	if len(supportedVersions) > 0 {
		supported := slices.Contains(supportedVersions, branch)
		rel.Supported = &supported
	}

	return rel, nil
}

// allSupportedVersions returns every supported PHP branch from php.net's
// latest release per major version, or nil if unavailable. Callers hold p.mu.
func (p *PHPNet) allSupportedVersions(client *http.Client) []string {
	if p.supported != nil {
		return p.supported
	}

	resp, err := client.Get(PHPReleasesURL + "?json")
	if err != nil {
		p.err = fmt.Errorf("supported PHP versions: %w", err)
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		p.err = fmt.Errorf("supported PHP versions: php.net returned %s", resp.Status)
		return nil
	}

	var majors map[string]phpNetResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&majors); err != nil {
		p.err = fmt.Errorf("supported PHP versions: %w", err)
		return nil
	}

	supported := []string{}
	for _, release := range majors {
		supported = append(supported, release.SupportedVersions...)
	}
	p.supported = supported

	return supported
}

func parsePHPNetDate(s string) (time.Time, error) {
	for _, layout := range []string{"02 Jan 2006", "2 Jan 2006"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized php.net date %q", s)
}

// Err returns the last php.net lookup failure, if any. When set, some PHP
// installs weren't compared with php.net's latest releases.
func (p *PHPNet) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}
