// Package state models what is observed on a host and what was last
// applied to it. It is plain data shared by observe (which fills it) and
// plan (which diffs against it).
package state

import (
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/roots/trellis-cli/pkg/converge/manifest"
)

const (
	Dir            = "/var/lib/trellis"
	CurrentPath    = Dir + "/current.json"
	GenerationsDir = Dir + "/generations"
	LockPath       = Dir + "/lock"
	// DeployLockPath is the lock deploys (as web) share with provisioning.
	DeployLockPath     = Dir + "/deploy.lock"
	RebootRequiredPath = "/var/run/reboot-required"
)

// FileInfo is the observed state of a path.
type FileInfo struct {
	Exists bool        `json:"exists"`
	IsDir  bool        `json:"is_dir,omitempty"`
	Mode   fs.FileMode `json:"mode,omitempty"`
	Owner  string      `json:"owner,omitempty"`
	Group  string      `json:"group,omitempty"`
	SHA256 string      `json:"sha256,omitempty"`
}

// Unit is the observed state of a systemd unit.
type Unit struct {
	LoadState     string `json:"load_state"`      // loaded, not-found, ...
	ActiveState   string `json:"active_state"`    // active, inactive, failed, ...
	UnitFileState string `json:"unit_file_state"` // enabled, disabled, static, generated, ...
}

func (u Unit) Active() bool { return u.ActiveState == "active" || u.ActiveState == "reloading" }
func (u Unit) Enabled() bool {
	return u.UnitFileState == "enabled" || u.UnitFileState == "enabled-runtime"
}

// Observed is a snapshot of everything the planner needs.
type Observed struct {
	Files     map[string]FileInfo `json:"files"`
	Packages  map[string]string   `json:"packages"` // installed name → version
	Units     map[string]Unit     `json:"units"`
	Databases map[string]bool     `json:"databases"` // by resource name
	DBUsers   map[string]bool     `json:"db_users"`  // by resource name
	// DBUnknown is set when databases could not be queried (e.g. MariaDB
	// isn't installed yet); the planner then treats them as missing, which
	// is safe because database statements are idempotent.
	DBUnknown bool        `json:"db_unknown,omitempty"`
	Previous  *Generation `json:"previous,omitempty"`
	// RebootRequired is set when updates are waiting for a reboot
	// (/var/run/reboot-required).
	RebootRequired bool `json:"reboot_required,omitempty"`
	// Upgradable are installed packages with an update waiting, as of the
	// server's last apt update.
	Upgradable []string `json:"upgradable,omitempty"`
}

// NewObserved returns an empty snapshot.
func NewObserved() *Observed {
	return &Observed{
		Files:     map[string]FileInfo{},
		Packages:  map[string]string{},
		Units:     map[string]Unit{},
		Databases: map[string]bool{},
		DBUsers:   map[string]bool{},
	}
}

// Generation records an apply. Manifest holds resources known to be in
// their desired state at the end of the run (their digests are trusted for
// change detection). Pending holds resources that were attempted but not
// confirmed (a failed run); they are still owned, so pruning sees them,
// but their digests are not trusted.
type Generation struct {
	Number   int                 `json:"number"`
	Failed   bool                `json:"failed,omitempty"`
	Manifest *manifest.Manifest  `json:"manifest"`
	Pending  []manifest.Resource `json:"pending,omitempty"`
}

// Applied returns the trusted record of id, or nil.
func (g *Generation) Applied(id manifest.ID) *manifest.Resource {
	if g == nil || g.Manifest == nil {
		return nil
	}
	return g.Manifest.Get(id)
}

// Owned returns every resource Trellis may have created, keyed by ID.
func (g *Generation) Owned() map[manifest.ID]manifest.Resource {
	out := map[manifest.ID]manifest.Resource{}
	if g == nil {
		return out
	}
	for _, r := range g.Pending {
		out[r.ID()] = r
	}
	if g.Manifest != nil {
		for _, r := range g.Manifest.Resources {
			out[r.ID()] = r
		}
	}
	return out
}

func (g *Generation) Marshal() ([]byte, error) { return json.Marshal(g) }

func UnmarshalGeneration(b []byte) (*Generation, error) {
	var g Generation
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, fmt.Errorf("decode generation: %w", err)
	}
	if g.Manifest == nil {
		g.Manifest = manifest.New("", nil)
	}
	return &g, nil
}

// GenerationPath is where generation n is archived.
func GenerationPath(n int) string {
	return fmt.Sprintf("%s/%06d.json", GenerationsDir, n)
}
