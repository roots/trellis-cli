package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"time"
)

// phpNet looks up PHP releases for the PHP service.
var phpNet = &phpNetClient{url: "https://www.php.net/releases/index.php"}

// phpNetClient looks up the latest PHP releases from php.net's releases API.
// Each branch is fetched at most once.
type phpNetClient struct {
	url     string
	client  *http.Client
	lookups map[string]phpNetLookup
}

type phpNetLookup struct {
	release *Release
	err     error
}

type phpNetResponse struct {
	Error             string   `json:"error"`
	Version           string   `json:"version"`
	Date              string   `json:"date"`
	Tags              []string `json:"tags"`
	SupportedVersions []string `json:"supported_versions"`
}

// latest returns the latest release of a PHP branch (e.g. "8.3"), or nil if
// php.net doesn't know the branch.
func (p *phpNetClient) latest(branch string) (*Release, error) {
	if l, ok := p.lookups[branch]; ok {
		return l.release, l.err
	}

	rel, err := p.fetch(branch)

	if p.lookups == nil {
		p.lookups = map[string]phpNetLookup{}
	}
	p.lookups[branch] = phpNetLookup{rel, err}

	return rel, err
}

func (p *phpNetClient) fetch(branch string) (*Release, error) {
	client := p.client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	resp, err := client.Get(fmt.Sprintf("%s?json&version=%s", p.url, url.QueryEscape(branch)))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("php.net returned %s", resp.Status)
	}

	var data phpNetResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&data); err != nil {
		return nil, err
	}

	// Unknown branches return 200 with {"error": "Unknown version"}, which
	// isn't a lookup failure.
	if data.Error != "" {
		return nil, nil
	}

	if branchOf(data.Version) != branch {
		return nil, fmt.Errorf("php.net returned %q for branch %s", data.Version, branch)
	}

	date, err := time.Parse("2 Jan 2006", data.Date)
	if err != nil {
		return nil, fmt.Errorf("unrecognized php.net date %q", data.Date)
	}

	rel := &Release{
		Version:  data.Version,
		Date:     date,
		Security: slices.Contains(data.Tags, "security"),
	}

	// The list is empty once a major version is entirely unsupported; leave
	// support unknown then.
	if len(data.SupportedVersions) > 0 {
		supported := slices.Contains(data.SupportedVersions, branch)
		rel.Supported = &supported
	}

	return rel, nil
}
