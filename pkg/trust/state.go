package trust

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/roots/trellis-cli/app_paths"
)

// Entry records what a `vm trust` invocation added so that a later
// `vm untrust` can reverse exactly the same set, even when the cert on disk
// has since changed.
type Entry struct {
	Project         string    `json:"project"`
	Site            string    `json:"site"`
	Fingerprint     string    `json:"fingerprint"`
	FingerprintSHA1 string    `json:"fingerprint_sha1"`
	CommonName      string    `json:"common_name,omitempty"`
	CertPath        string    `json:"cert_path"`
	KeyPath         string    `json:"key_path,omitempty"`
	Label           string    `json:"label"`
	Locations       []string  `json:"locations"`
	AddedAt         time.Time `json:"added_at"`
}

type State struct {
	Entries []Entry `json:"entries"`
}

func StatePath() string {
	return filepath.Join(app_paths.DataDir(), "state", "trusted_certs.json")
}

func Load() (*State, error) {
	state := &State{}

	data, err := os.ReadFile(StatePath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return state, nil
		}
		return nil, err
	}

	if len(data) == 0 {
		return state, nil
	}

	if err := json.Unmarshal(data, state); err != nil {
		return nil, fmt.Errorf("trust state file %s is corrupt (%w). Move it aside and re-run `trellis vm trust` to rebuild", StatePath(), err)
	}

	return state, nil
}

func (s *State) Save() error {
	path := StatePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".trusted_certs.*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Find returns the entry matching project + site, or nil if absent.
func (s *State) Find(project, site string) *Entry {
	for i := range s.Entries {
		if s.Entries[i].Project == project && s.Entries[i].Site == site {
			return &s.Entries[i]
		}
	}
	return nil
}

// Upsert replaces an existing project+site entry or appends a new one.
func (s *State) Upsert(entry Entry) {
	for i := range s.Entries {
		if s.Entries[i].Project == entry.Project && s.Entries[i].Site == entry.Site {
			s.Entries[i] = entry
			return
		}
	}
	s.Entries = append(s.Entries, entry)
	sort.SliceStable(s.Entries, func(i, j int) bool {
		if s.Entries[i].Project == s.Entries[j].Project {
			return s.Entries[i].Site < s.Entries[j].Site
		}
		return s.Entries[i].Project < s.Entries[j].Project
	})
}

// Remove drops the entry for project+site (no-op if absent).
func (s *State) Remove(project, site string) {
	out := s.Entries[:0]
	for _, e := range s.Entries {
		if e.Project == project && e.Site == site {
			continue
		}
		out = append(out, e)
	}
	s.Entries = out
}

// EntriesForProject returns entries belonging to a project, sorted by site.
func (s *State) EntriesForProject(project string) []Entry {
	var out []Entry
	for _, e := range s.Entries {
		if e.Project == project {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Site < out[j].Site })
	return out
}
