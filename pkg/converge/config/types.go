// Package config loads trellis.yml and resolves it into a fully merged,
// defaulted and validated Environment.
//
// There is exactly one merge rule:
//
//	defaults → environments.<env>            (server settings)
//	sites.<site> → environments.<env>.sites.<site>  (site settings)
//
// Maps deep-merge, lists replace, and an explicit null removes a key (or,
// for a site under an environment, excludes the site from it).
package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// File is the raw shape of trellis.yml. It is decoded strictly (unknown
// keys are errors) before merging, so typos are reported with line numbers.
type File struct {
	Version      int                     `yaml:"version"`
	Defaults     Server                  `yaml:"defaults"`
	Access       Access                  `yaml:"access"`
	Sites        map[string]Site         `yaml:"sites"`
	Environments map[string]*Environment `yaml:"environments"`
}

// Environment is both the raw per-environment block and, after Resolve, the
// fully merged result.
type Environment struct {
	Name     string            `yaml:"-"`
	Server   Server            `yaml:",inline"` // server settings override `defaults`
	Access   Access            `yaml:"access,omitempty"`
	Nodes    []Node            `yaml:"nodes,omitempty"`
	Database *ExternalDatabase `yaml:"database,omitempty"`

	// RawSites holds per-environment site overrides (nil value = excluded).
	RawSites map[string]*Site `yaml:"sites,omitempty"`

	// Sites is populated by Resolve: merged, defaulted, sorted by name.
	Sites []Site `yaml:"-"`
}

// Server holds node-level settings shared by every site.
type Server struct {
	Timezone string `yaml:"timezone,omitempty"`
	// Swap is "auto" (a swapfile of min(RAM, 2G) if the server has no
	// swap), a size for /swapfile (e.g. "1G"), or "off".
	Swap     string             `yaml:"swap,omitempty"`
	PHP      PHP                `yaml:"php,omitempty"`
	MariaDB  map[string]string  `yaml:"mariadb,omitempty"`
	Mail     *Mail              `yaml:"mail,omitempty"`
	Security Security           `yaml:"security,omitempty"`
	Packages []string           `yaml:"packages,omitempty"`
	Services map[string]Service `yaml:"services,omitempty"`
	Exec     []Exec             `yaml:"exec,omitempty"`
}

type PHP struct {
	Version string            `yaml:"version,omitempty"`
	INI     map[string]string `yaml:"ini,omitempty"`
	FPM     FPM               `yaml:"fpm,omitempty"`
}

type FPM struct {
	PM              string `yaml:"pm,omitempty"`
	MaxChildren     int    `yaml:"max_children,omitempty"`
	StartServers    int    `yaml:"start_servers,omitempty"`
	MinSpareServers int    `yaml:"min_spare_servers,omitempty"`
	MaxSpareServers int    `yaml:"max_spare_servers,omitempty"`
	MaxRequests     int    `yaml:"max_requests,omitempty"`
}

type Mail struct {
	SMTP     string `yaml:"smtp"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
	From     string `yaml:"from,omitempty"`
}

type Security struct {
	SSH      SSH      `yaml:"ssh,omitempty"`
	Fail2ban Fail2ban `yaml:"fail2ban,omitempty"`
}

type SSH struct {
	AllowFrom []string `yaml:"allow_from,omitempty"`
	RateLimit *bool    `yaml:"rate_limit,omitempty"`
}

type Fail2ban struct {
	WPLogin bool `yaml:"wp_login,omitempty"`
	XMLRPC  bool `yaml:"xmlrpc,omitempty"`
}

// Service is a Podman Quadlet container. The image tag is resolved to a
// digest in trellis.lock.
type Service struct {
	Image   string            `yaml:"image"`
	Publish []string          `yaml:"publish,omitempty"`
	Env     map[string]string `yaml:"env,omitempty"`
	Volumes []string          `yaml:"volumes,omitempty"`
	Memory  string            `yaml:"memory,omitempty"`
	Health  *Health           `yaml:"health,omitempty"`
}

type Health struct {
	HTTP string `yaml:"http,omitempty"`
}

// Exec is the escape hatch: a shell command re-run only when the hash of
// its inputs (project paths) changes.
type Exec struct {
	Name   string   `yaml:"name"`
	Run    string   `yaml:"run"`
	Inputs []string `yaml:"inputs,omitempty"`
}

// Access lists SSH key references: "github:<user>", "file:<path>" or a
// literal public key.
type Access struct {
	// Admins get a sudo login and can decrypt the environment's secrets
	// (their ed25519/RSA SSH keys are the age recipients).
	Admins []string `yaml:"admins,omitempty"`
	Deploy []string `yaml:"deploy,omitempty"`
	// Recipients are extra age public keys (age1…) or key references that
	// can decrypt secrets without server access, e.g. CI.
	Recipients []string `yaml:"recipients,omitempty"`
}

const (
	RoleWeb = "web"
	RoleDB  = "db"
)

type Node struct {
	Name      string            `yaml:"name,omitempty"`
	Host      string            `yaml:"host"`
	PrivateIP string            `yaml:"private_ip,omitempty"`
	User      string            `yaml:"user,omitempty"` // default: try admin, then root
	Port      int               `yaml:"port,omitempty"`
	Roles     []string          `yaml:"roles,omitempty"`
	MariaDB   map[string]string `yaml:"mariadb,omitempty"`
}

func (n Node) HasRole(role string) bool {
	for _, r := range n.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// Address is how other nodes reach this one (private IP preferred).
func (n Node) Address() string {
	if n.PrivateIP != "" {
		return n.PrivateIP
	}
	return n.Host
}

type ExternalDatabase struct {
	Host  string   `yaml:"host"`
	Port  int      `yaml:"port,omitempty"`
	TLS   string   `yaml:"tls,omitempty"` // "required" or ""
	Admin *DBAdmin `yaml:"admin,omitempty"`
}

type DBAdmin struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

const (
	SSLLetsEncrypt = "letsencrypt"
	SSLSelfSigned  = "self-signed"
	SSLNone        = "none"

	CacheOff     = "off"
	CacheFastCGI = "fastcgi"

	ObjectCacheNone      = "none"
	ObjectCacheRedis     = "redis"
	ObjectCacheMemcached = "memcached"
)

type Site struct {
	Name        string            `yaml:"-"`
	EnvName     string            `yaml:"-"` // the environment it was resolved for
	Repo        string            `yaml:"repo,omitempty"`
	Path        string            `yaml:"path,omitempty"`
	Branch      string            `yaml:"branch,omitempty"`
	LocalPath   string            `yaml:"local_path,omitempty"`
	Domain      string            `yaml:"domain,omitempty"`
	Redirects   []string          `yaml:"redirects,omitempty"`
	SSL         string            `yaml:"ssl,omitempty"`
	Cache       string            `yaml:"cache,omitempty"`
	ObjectCache string            `yaml:"object_cache,omitempty"`
	XMLRPC      bool              `yaml:"xmlrpc,omitempty"`
	Multisite   Multisite         `yaml:"multisite,omitempty"`
	Writable    []string          `yaml:"writable,omitempty"`
	PHP         SitePHP           `yaml:"php,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	DB          SiteDB            `yaml:"db,omitempty"`
}

type SitePHP struct {
	INI map[string]string `yaml:"ini,omitempty"`
}

type SiteDB struct {
	Name string `yaml:"name,omitempty"`
	User string `yaml:"user,omitempty"`
}

// Multisite accepts `false`, `true` (subdirectories) or
// `{mode: subdomains|subdirectories}`.
type Multisite struct {
	Enabled bool
	Mode    string // "subdomains" | "subdirectories"
}

func (m *Multisite) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var b bool
		if err := n.Decode(&b); err != nil {
			return fmt.Errorf("line %d: multisite must be a bool or {mode: ...}", n.Line)
		}
		*m = Multisite{Enabled: b}
		if b {
			m.Mode = "subdirectories"
		}
		return nil
	}
	var raw struct {
		Mode string `yaml:"mode"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	if raw.Mode != "subdomains" && raw.Mode != "subdirectories" {
		return fmt.Errorf("line %d: multisite.mode must be subdomains or subdirectories", n.Line)
	}
	*m = Multisite{Enabled: true, Mode: raw.Mode}
	return nil
}

func (m Multisite) MarshalYAML() (any, error) {
	if !m.Enabled {
		return false, nil
	}
	return map[string]string{"mode": m.Mode}, nil
}

// Domains returns the canonical domain followed by redirects.
func (s Site) Domains() []string {
	return append([]string{s.Domain}, s.Redirects...)
}

// DBName is the site's database name (default: site name with dots and
// dashes replaced by underscores).
// DBName is the site's database: <site>_<environment>, as Trellis has
// always named it, so existing servers keep their data.
func (s Site) DBName() string {
	if s.DB.Name != "" {
		return s.DB.Name
	}
	if s.EnvName == "" {
		return identifier(s.Name)
	}
	return identifier(s.Name) + "_" + identifier(s.EnvName)
}

// DBUser is the site's database user (default: same as DBName, truncated to
// MariaDB's 80 char limit... in practice 32 for compatibility).
func (s Site) DBUser() string {
	if s.DB.User != "" {
		return s.DB.User
	}
	u := identifier(s.Name)
	if len(u) > 32 {
		u = u[:32]
	}
	return u
}

func identifier(s string) string {
	b := []byte(s)
	for i, c := range b {
		alnum := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
		if !alnum && c != '_' {
			b[i] = '_'
		}
	}
	return string(b)
}
