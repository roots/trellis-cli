// Package manifest defines the fully rendered desired state of one node.
//
// A Manifest is the boundary between the compiler (a pure function of
// trellis.yml + trellis.lock + secrets + project files) and the engine
// (plan/apply on the host). It is plain data: every file's bytes, every
// package, unit, database and command, plus the notifications that fire
// when a resource changes. Nothing in a manifest is evaluated on the host.
//
// The resource set is deliberately closed: a fixed list of kinds keeps the
// compiler, planner and executor small and makes manifests easy to reason
// about (and to generate correctly with an LLM).
package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"slices"
	"strings"
)

// Kind identifies a resource type.
type Kind string

const (
	KindPackage      Kind = "package"
	KindFile         Kind = "file"
	KindDownload     Kind = "download"
	KindService      Kind = "service"
	KindDatabase     Kind = "database"
	KindDatabaseUser Kind = "database_user"
	KindCert         Kind = "cert"
	KindExec         Kind = "exec"
)

// Phase orders resources during apply. Within a phase resources are applied
// in ID order. Phase-end commands (see Notify) run when a phase finishes.
type Phase int

const (
	PhaseRepos    Phase = iota // apt keys + sources; ends with apt-get update
	PhasePackages              // one apt-get transaction
	PhaseIdentity              // sysusers.d / tmpfiles.d; ends with systemd-sysusers/tmpfiles
	PhaseFiles                 // configuration files
	PhaseCerts                 // TLS material (must exist before services start)
	PhaseData                  // databases and database users
	PhaseServices              // unit enable/active state
	PhaseExec                  // guarded user commands
	numPhases
)

// Phases returns all phases in apply order.
func Phases() []Phase {
	p := make([]Phase, numPhases)
	for i := range p {
		p[i] = Phase(i)
	}
	return p
}

func (p Phase) String() string {
	switch p {
	case PhaseRepos:
		return "repos"
	case PhasePackages:
		return "packages"
	case PhaseIdentity:
		return "identity"
	case PhaseFiles:
		return "files"
	case PhaseCerts:
		return "certs"
	case PhaseData:
		return "data"
	case PhaseServices:
		return "services"
	case PhaseExec:
		return "exec"
	}
	return fmt.Sprintf("phase(%d)", int(p))
}

// ID uniquely identifies a resource within a manifest. Two resources with
// the same ID are an ownership conflict.
type ID struct {
	Kind Kind
	Name string
}

func (id ID) String() string { return string(id.Kind) + ":" + id.Name }

// ParseID is the inverse of ID.String.
func ParseID(s string) (ID, error) {
	kind, name, ok := strings.Cut(s, ":")
	if !ok || name == "" {
		return ID{}, fmt.Errorf("invalid resource id %q", s)
	}
	return ID{Kind: Kind(kind), Name: name}, nil
}

// UnitAction is what a notification does to a systemd unit.
type UnitAction string

const (
	Reload  UnitAction = "reload"
	Restart UnitAction = "restart"
)

// Notify describes a side effect triggered when a resource changes.
//
// Exactly one of Command or Unit is set:
//   - Command runs once at the end of the resource's phase (deduplicated),
//     e.g. `apt-get update`, `systemd-sysusers`, `systemctl daemon-reload`.
//   - Unit+Action is deferred to the end of the run (deduplicated; restart
//     wins over reload) and skipped if the unit was started in this run.
//
// Check, if set, must succeed before the command/action runs. A failed
// check aborts the run and restores the files changed in it.
type Notify struct {
	Command []string   `json:"command,omitempty"`
	Unit    string     `json:"unit,omitempty"`
	Action  UnitAction `json:"action,omitempty"`
	Check   []string   `json:"check,omitempty"`
}

// Key identifies a notification for deduplication.
func (n Notify) Key() string {
	if n.Unit != "" {
		return "unit:" + n.Unit
	}
	return "cmd:" + strings.Join(n.Command, "\x00")
}

// Resource is a tagged union: exactly one of the kind-specific pointers is
// set, matching Kind. A flat struct (rather than an interface) keeps JSON
// encoding trivial and canonical, which the content hash depends on.
type Resource struct {
	Kind   Kind     `json:"kind"`
	Name   string   `json:"name"`
	Phase  Phase    `json:"phase"`
	Notify []Notify `json:"notify,omitempty"`

	Package      *Package      `json:"package,omitempty"`
	File         *File         `json:"file,omitempty"`
	Download     *Download     `json:"download,omitempty"`
	Service      *Service      `json:"service,omitempty"`
	Database     *Database     `json:"database,omitempty"`
	DatabaseUser *DatabaseUser `json:"database_user,omitempty"`
	Cert         *Cert         `json:"cert,omitempty"`
	Exec         *Exec         `json:"exec,omitempty"`
}

func (r *Resource) ID() ID { return ID{Kind: r.Kind, Name: r.Name} }

// Digest is a stable content hash of the resource (including notifications).
// It is used to detect changes for resources whose live state cannot be read
// back cheaply (database user passwords, exec inputs, cert domains). It is
// computed over the redacted form, so it is the same for a resource and
// its redacted copy.
func (r *Resource) Digest() string {
	red := r.Redacted()
	b, err := json.Marshal(&red)
	if err != nil {
		// Resource contains only JSON-safe types; this cannot happen.
		panic(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Sensitive reports whether the resource carries secret material that must
// not be printed (file contents, passwords).
func (r *Resource) Sensitive() bool {
	switch r.Kind {
	case KindFile:
		return r.File.Sensitive
	case KindDatabaseUser:
		return true
	case KindExec:
		return r.Exec.Stdin != "" || len(r.Exec.Env) > 0
	}
	return false
}

func (r *Resource) validate() error {
	set := 0
	for _, p := range []bool{
		r.Package != nil, r.File != nil, r.Download != nil, r.Service != nil,
		r.Database != nil, r.DatabaseUser != nil, r.Cert != nil, r.Exec != nil,
	} {
		if p {
			set++
		}
	}
	if set != 1 {
		return fmt.Errorf("%s: resource must set exactly one spec, has %d", r.ID(), set)
	}
	ok := false
	switch r.Kind {
	case KindPackage:
		ok = r.Package != nil
	case KindFile:
		ok = r.File != nil
	case KindDownload:
		ok = r.Download != nil
	case KindService:
		ok = r.Service != nil
	case KindDatabase:
		ok = r.Database != nil
	case KindDatabaseUser:
		ok = r.DatabaseUser != nil
	case KindCert:
		ok = r.Cert != nil
	case KindExec:
		ok = r.Exec != nil
	default:
		return fmt.Errorf("%s: unknown kind", r.ID())
	}
	if !ok {
		return fmt.Errorf("%s: spec does not match kind", r.ID())
	}
	if r.Phase < 0 || r.Phase >= numPhases {
		return fmt.Errorf("%s: invalid phase %d", r.ID(), r.Phase)
	}
	for _, n := range r.Notify {
		if (n.Unit == "") == (len(n.Command) == 0) {
			return fmt.Errorf("%s: notify must set exactly one of unit or command", r.ID())
		}
		if n.Unit != "" && n.Action != Reload && n.Action != Restart {
			return fmt.Errorf("%s: notify %s has invalid action %q", r.ID(), n.Unit, n.Action)
		}
	}
	return nil
}

// Package is an apt package. Version pins an exact version when set.
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// File is a whole file owned by Trellis. There are no partial edits.
type File struct {
	Path      string      `json:"path"`
	Content   string      `json:"content,omitempty"`
	Mode      fs.FileMode `json:"mode"`
	Owner     string      `json:"owner"`
	Group     string      `json:"group"`
	Absent    bool        `json:"absent,omitempty"`    // ensure the path does not exist
	Sensitive bool        `json:"sensitive,omitempty"` // never print content
	// Validate runs against the staged file before it replaces the live
	// one; "%s" is replaced by the staged path (e.g. visudo -cf %s). A
	// failure leaves the live file untouched.
	Validate []string `json:"validate,omitempty"`
}

// SHA256 returns the hex sha256 of the desired content.
func (f *File) SHA256() string {
	sum := sha256.Sum256([]byte(f.Content))
	return hex.EncodeToString(sum[:])
}

// Download fetches a file once. If SHA256 is set the content must match;
// if Fingerprints is set the file must be OpenPGP keys, every one of them
// pinned (apt trusts every key in a keyring, so an extra key is refused).
type Download struct {
	Path         string      `json:"path"`
	URL          string      `json:"url"`
	SHA256       string      `json:"sha256,omitempty"`
	Fingerprints []string    `json:"fingerprints,omitempty"`
	Mode         fs.FileMode `json:"mode"`
}

// Service is the desired state of a systemd unit. Enabled is nil for units
// whose enablement is not managed through systemctl (e.g. Quadlet units,
// which are enabled by their generator).
type Service struct {
	Unit    string `json:"unit"`
	Enabled *bool  `json:"enabled,omitempty"`
	Active  bool   `json:"active"`
}

// DBServer is a remote database server. Nil means the local server over
// the unix socket as root.
type DBServer struct {
	Host     string `json:"host"`
	Port     int    `json:"port,omitempty"`
	User     string `json:"user"`
	Password string `json:"password"`
	TLS      bool   `json:"tls,omitempty"`
}

// Database is a MariaDB/MySQL database. Databases are never dropped by
// pruning; removing one from the manifest only produces a warning.
type Database struct {
	Name   string    `json:"name"`
	Server *DBServer `json:"server,omitempty"`
}

// DatabaseUser is an account with ALL privileges on Databases.
type DatabaseUser struct {
	User      string    `json:"user"`
	Host      string    `json:"host"`
	Password  string    `json:"password"`
	Databases []string  `json:"databases"`
	Server    *DBServer `json:"server,omitempty"`
}

// CertProvider is how TLS material is obtained.
type CertProvider string

const (
	CertSelfSigned  CertProvider = "self-signed"
	CertLetsEncrypt CertProvider = "letsencrypt"
)

// Cert is a TLS certificate + key for a set of domains.
type Cert struct {
	Provider CertProvider `json:"provider"`
	Domains  []string     `json:"domains"`
	CertPath string       `json:"cert_path"`
	KeyPath  string       `json:"key_path"`
	Email    string       `json:"email,omitempty"` // ACME account email
}

// Exec runs Command when its Inputs hash differs from the last applied
// generation (or it never ran). It is the escape hatch: always visible in
// plans.
type Exec struct {
	Command []string `json:"command"`
	Inputs  string   `json:"inputs"`
	// Stdin is secret input (e.g. a password for chpasswd). It is redacted
	// in history; a change to it re-runs the command.
	Stdin string `json:"stdin,omitempty"`
	// Env is passed to the command. Values are treated like Stdin:
	// redacted in history, and a change re-runs the command.
	Env map[string]string `json:"env,omitempty"`
}

// Manifest is the desired state of a single node.
type Manifest struct {
	Node      string     `json:"node"`
	Roles     []string   `json:"roles"`
	OS        string     `json:"os,omitempty"` // e.g. ubuntu-24.04; checked against the server
	Resources []Resource `json:"resources"`
	Warnings  []string   `json:"warnings,omitempty"`

	index map[ID]int
}

// New returns an empty manifest for node.
func New(node string, roles []string) *Manifest {
	return &Manifest{Node: node, Roles: roles, index: map[ID]int{}}
}

// Add appends resources. Adding a resource whose ID already exists is an
// ownership conflict and returns an error: every path, unit and package has
// exactly one owner.
func (m *Manifest) Add(rs ...Resource) error {
	if m.index == nil {
		m.reindex()
	}
	for _, r := range rs {
		if err := r.validate(); err != nil {
			return err
		}
		id := r.ID()
		if _, ok := m.index[id]; ok {
			return fmt.Errorf("ownership conflict: %s is declared more than once", id)
		}
		m.index[id] = len(m.Resources)
		m.Resources = append(m.Resources, r)
	}
	return nil
}

// Get returns the resource with id, or nil.
func (m *Manifest) Get(id ID) *Resource {
	if m.index == nil {
		m.reindex()
	}
	i, ok := m.index[id]
	if !ok {
		return nil
	}
	return &m.Resources[i]
}

func (m *Manifest) reindex() {
	m.index = make(map[ID]int, len(m.Resources))
	for i := range m.Resources {
		m.index[m.Resources[i].ID()] = i
	}
}

// Finalize sorts resources into canonical order (phase, then ID). It must
// be called before hashing or serializing so manifests are byte-identical
// regardless of the order the compiler added resources.
func (m *Manifest) Finalize() {
	slices.SortFunc(m.Resources, func(a, b Resource) int {
		if a.Phase != b.Phase {
			return int(a.Phase - b.Phase)
		}
		return strings.Compare(a.ID().String(), b.ID().String())
	})
	slices.Sort(m.Warnings)
	m.Warnings = slices.Compact(m.Warnings)
	m.reindex()
}

// Hash is the content hash of the whole manifest. Equal hashes mean equal
// desired state.
func (m *Manifest) Hash() string {
	h := sha256.New()
	for i := range m.Resources {
		h.Write([]byte(m.Resources[i].Digest()))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Validate checks every resource and the ownership invariant.
func (m *Manifest) Validate() error {
	seen := make(map[ID]bool, len(m.Resources))
	for i := range m.Resources {
		r := &m.Resources[i]
		if err := r.validate(); err != nil {
			return err
		}
		if seen[r.ID()] {
			return fmt.Errorf("ownership conflict: %s is declared more than once", r.ID())
		}
		seen[r.ID()] = true
	}
	return nil
}

// Marshal encodes the manifest as canonical JSON.
func (m *Manifest) Marshal() ([]byte, error) {
	return json.Marshal(m)
}

// Unmarshal decodes and validates a manifest.
func Unmarshal(b []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	m.reindex()
	return &m, nil
}
