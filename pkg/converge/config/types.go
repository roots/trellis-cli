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
	"strings"

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
	Name   string `yaml:"-"`
	Server Server `yaml:",inline"` // server settings override `defaults`
	Access Access `yaml:"access,omitempty"`
	// Host is the server (one per environment). Development may omit it
	// and target a VM with --lima or --host.
	Host     string            `yaml:"host,omitempty"`
	User     string            `yaml:"user,omitempty"` // SSH user for provisioning (default: root)
	Port     int               `yaml:"port,omitempty"` // SSH port (default 22)
	Database *ExternalDatabase `yaml:"database,omitempty"`

	// RawSites holds per-environment site overrides (nil value = excluded).
	RawSites map[string]*Site `yaml:"sites,omitempty"`

	// Sites is populated by Resolve: merged, defaulted, sorted by name.
	Sites []Site `yaml:"-"`
}

// Server holds node-level settings shared by every site.
type Server struct {
	// OS is the server's operating system, one of OSes (default
	// ubuntu-24.04). Plans refuse to run on a server with a different OS.
	OS       string `yaml:"os,omitempty"`
	Timezone string `yaml:"timezone,omitempty"`
	// Swap is "auto" (a swapfile of min(RAM, 2G) if the server has no
	// swap), a size for /swapfile (e.g. "1G"), or false.
	Swap Choice `yaml:"swap,omitempty"`
	// AutoUpdates is what unattended-upgrades installs daily: UpdatesAll
	// (default), UpdatesDistro or false.
	AutoUpdates Choice            `yaml:"auto_updates,omitempty"`
	PHP         PHP               `yaml:"php,omitempty"`
	MariaDB     map[string]string `yaml:"mariadb,omitempty"`
	Mail        *Mail             `yaml:"mail,omitempty"`
	SSH         SSH               `yaml:"ssh,omitempty"`
	Fail2ban    Fail2ban          `yaml:"fail2ban,omitempty"`
	Packages    []string          `yaml:"packages,omitempty"`
	Commands    []Command         `yaml:"commands,omitempty"`
}

// Off is what a Choice holds for false.
const Off = "off"

// Choice is a setting that names one of a few options, or is false to turn
// it off (object_cache: redis, swap: false). Words like off and none are
// refused, so there's one way to say it.
type Choice string

func (c *Choice) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: expected a value or false", n.Line)
	}
	if n.ShortTag() == "!!bool" {
		var on bool
		if err := n.Decode(&on); err != nil {
			return err
		}
		if on {
			return fmt.Errorf("line %d: true isn't one of the options here (false turns it off)", n.Line)
		}
		*c = Off
		return nil
	}
	switch strings.ToLower(n.Value) {
	case "off", "none", "no", "disabled":
		return fmt.Errorf("line %d: use false to turn this off", n.Line)
	}
	*c = Choice(n.Value)
	return nil
}

func (c Choice) MarshalYAML() (any, error) {
	if c == Off {
		return false, nil
	}
	return string(c), nil
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

// Or fills f's unset fields from def.
func (f FPM) Or(def FPM) FPM {
	if f.PM == "" {
		f.PM = def.PM
	}
	for _, p := range []struct{ v, d *int }{
		{&f.MaxChildren, &def.MaxChildren}, {&f.StartServers, &def.StartServers},
		{&f.MinSpareServers, &def.MinSpareServers}, {&f.MaxSpareServers, &def.MaxSpareServers},
		{&f.MaxRequests, &def.MaxRequests},
	} {
		if *p.v == 0 {
			*p.v = *p.d
		}
	}
	return f
}

type Mail struct {
	SMTP     string `yaml:"smtp"`
	User     string `yaml:"user,omitempty"`
	Password string `yaml:"password,omitempty"`
	From     string `yaml:"from,omitempty"`
}

type SSH struct {
	AllowFrom []string `yaml:"allow_from,omitempty"`
	RateLimit *bool    `yaml:"rate_limit,omitempty"`
}

// Fail2ban tunes the WordPress jails. sshd's is always on, and the
// xmlrpc.php one follows the sites: on when any site allows XML-RPC
// (nginx refuses it otherwise).
type Fail2ban struct {
	// WPLogin bans IPs that keep failing WordPress logins (default true).
	// Turn it off behind a proxy nginx doesn't see real client IPs through.
	WPLogin *bool `yaml:"wp_login,omitempty"`
}

// Command is the escape hatch for what files and packages can't express:
// a shell command run as root after everything else. It runs once, then
// again only when run, env, or a file it watches (on) changes. Commands
// are never undone.
type Command struct {
	// Name identifies the command in plans and history (default: derived
	// from run).
	Name string `yaml:"name,omitempty"`
	Run  string `yaml:"run"`
	// On lists server paths (files or directories, e.g. /usr/local/share/fonts/)
	// managed by this manifest; a change to any of them re-runs the command.
	On []string `yaml:"on,omitempty"`
	// Env is passed to the command; values may be secret:<path>. Values are
	// kept out of plans and history.
	Env map[string]string `yaml:"env,omitempty"`
}

// CommandName is the command's name, or one derived from run: its first
// few words, e.g. "fc-cache -f" → "fc-cache-f".
func (c Command) CommandName() string {
	if c.Name != "" {
		return c.Name
	}
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(c.Run) {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_'
		switch {
		case ok:
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		default:
			dash = true
		}
		if b.Len() >= 40 {
			break
		}
	}
	return b.String()
}

// Access lists SSH key references: "github:<user>", "file:<path>" or a
// literal public key.
type Access struct {
	// Admins log in as admin (passwordless sudo) or root, and can decrypt
	// the environment's secrets (their ed25519/RSA SSH keys are the age
	// recipients).
	Admins []string `yaml:"admins,omitempty"`
	Deploy []string `yaml:"deploy,omitempty"`
	// Recipients are extra age public keys (age1…) or key references that
	// can decrypt secrets without server access, e.g. CI.
	Recipients []string `yaml:"recipients,omitempty"`
}

// ServerName names the environment's server in output: its host, or the
// environment for a development VM without one.
func (e *Environment) ServerName() string {
	if e.Host != "" {
		return e.Host
	}
	return e.Name
}

// LocalDB reports whether MariaDB runs on the server (no external database).
func (e *Environment) LocalDB() bool { return e.Database == nil }

type ExternalDatabase struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port,omitempty"`
	// TLS encrypts connections (default true: a managed database is
	// reached over the network).
	TLS   *bool    `yaml:"tls,omitempty"`
	Admin *DBAdmin `yaml:"admin,omitempty"`
}

// UsesTLS reports whether connections to the database use TLS.
func (d ExternalDatabase) UsesTLS() bool { return d.TLS == nil || *d.TLS }

type DBAdmin struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

const (
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
	HTTPS       HTTPS             `yaml:"https,omitempty"`
	Cache       bool              `yaml:"cache,omitempty"` // nginx FastCGI page cache
	ObjectCache Choice            `yaml:"object_cache,omitempty"`
	XMLRPC      bool              `yaml:"xmlrpc,omitempty"`
	Multisite   Multisite         `yaml:"multisite,omitempty"`
	Writable    []string          `yaml:"writable,omitempty"`
	PHP         SitePHP           `yaml:"php,omitempty"`
	Env         map[string]string `yaml:"env,omitempty"`
	DB          SiteDB            `yaml:"db,omitempty"`
	Deploy      SiteDeploy        `yaml:"deploy,omitempty"`
}

// SiteDeploy tunes deploys of a site.
type SiteDeploy struct {
	Keep     int   `yaml:"keep,omitempty"`      // releases kept on servers (default 5)
	UpdateDB *bool `yaml:"update_db,omitempty"` // wp core update-db after the switch (default true)
	// Exclude keeps paths out of the release: a pattern without a slash
	// matches any file or directory name (e.g. "*.map"); one with a slash
	// matches a path from the release root (e.g. "web/app/themes/*/resources").
	Exclude []string `yaml:"exclude,omitempty"`
}

// UpdatesDB reports whether deploys run wp core update-db.
func (d SiteDeploy) UpdatesDB() bool { return d.UpdateDB == nil || *d.UpdateDB }

type SitePHP struct {
	INI map[string]string `yaml:"ini,omitempty"`
	// FPM overrides the server's php.fpm for this site's pool; unset
	// fields come from it.
	FPM FPM `yaml:"fpm,omitempty"`
}

type SiteDB struct {
	Name string `yaml:"name,omitempty"`
	User string `yaml:"user,omitempty"`
}

// HTTPS modes, after resolving (see HTTPS).
const (
	HTTPSLetsEncrypt = "letsencrypt"
	HTTPSSelfSigned  = "self-signed"
	HTTPSOff         = "off"
	HTTPSCustom      = "custom"
)

// HTTPS is a site's `https:` setting. Omitted or true is an automatic
// certificate (Let's Encrypt; self-signed in development), false serves
// plain HTTP, "self-signed" a self-signed certificate, and
// {certificate: file:<path>, key: secret:<path>} the site's own.
type HTTPS struct {
	// Mode is "" until resolved (automatic), then one of the HTTPS* modes.
	Mode        string
	Certificate string // file:<path> in the project (HTTPSCustom)
	Key         string // secret:<path> (HTTPSCustom)
}

func (h *HTTPS) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		if n.Value == HTTPSSelfSigned {
			*h = HTTPS{Mode: HTTPSSelfSigned}
			return nil
		}
		var on bool
		if err := n.Decode(&on); err != nil {
			return fmt.Errorf("line %d: https must be true, false, self-signed or {certificate: file:<path>, key: secret:<path>}", n.Line)
		}
		*h = HTTPS{}
		if !on {
			h.Mode = HTTPSOff
		}
		return nil
	}
	var raw struct {
		Certificate string `yaml:"certificate"`
		Key         string `yaml:"key"`
	}
	if err := n.Decode(&raw); err != nil {
		return err
	}
	*h = HTTPS{Mode: HTTPSCustom, Certificate: raw.Certificate, Key: raw.Key}
	return nil
}

func (h HTTPS) MarshalYAML() (any, error) {
	switch h.Mode {
	case "", HTTPSLetsEncrypt:
		return true, nil
	case HTTPSOff:
		return false, nil
	case HTTPSCustom:
		return map[string]string{"certificate": h.Certificate, "key": h.Key}, nil
	}
	return h.Mode, nil
}

// On reports whether the site serves HTTPS.
func (h HTTPS) On() bool { return h.Mode != HTTPSOff }

// Multisite accepts false, true (subdirectories), subdomains or
// subdirectories.
type Multisite struct {
	Enabled bool
	Mode    string // "subdomains" | "subdirectories"
}

func (m *Multisite) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		switch n.Value {
		case "subdomains", "subdirectories":
			*m = Multisite{Enabled: true, Mode: n.Value}
			return nil
		}
		var b bool
		if err := n.Decode(&b); err == nil {
			*m = Multisite{Enabled: b}
			if b {
				m.Mode = "subdirectories"
			}
			return nil
		}
	}
	return fmt.Errorf("line %d: multisite must be true, false, subdomains or subdirectories", n.Line)
}

func (m Multisite) MarshalYAML() (any, error) {
	if !m.Enabled {
		return false, nil
	}
	return m.Mode, nil
}

// Domains returns the canonical domain followed by redirects.
func (s Site) Domains() []string {
	return append([]string{s.Domain}, s.Redirects...)
}

// DBName is the site's database: <site>_<environment>, as Trellis has
// always named it, so servers converge takes over keep their data.
func (s Site) DBName() string {
	if s.DB.Name != "" {
		return s.DB.Name
	}
	if s.EnvName == "" {
		return identifier(s.Name)
	}
	return identifier(s.Name) + "_" + identifier(s.EnvName)
}

// DBUser is the site's database user (default: the site name, cut to 32
// characters, MySQL's limit).
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

// SharedName is the directory under shared/ that a writable release path
// (e.g. web/app/uploads) is linked to: its last path element.
func SharedName(p string) string {
	p = strings.Trim(p, "/")
	return p[strings.LastIndex(p, "/")+1:]
}

// AutoUpdates values.
const (
	// UpdatesAll installs the distro's security and bugfix updates, and
	// bugfix and security releases from the repositories Trellis adds
	// (nginx, PHP, MariaDB within its series).
	UpdatesAll = "all"
	// UpdatesDistro keeps the distro's default: its security updates only
	// (and Debian's point releases).
	UpdatesDistro = "distro"
	// UpdatesOff (false) installs nothing automatically; `trellis
	// converge upgrade` installs what UpdatesAll would.
	UpdatesOff = Off
)

// OS is a supported operating system. Name is what trellis.yml's os: says
// and the server's /etc/os-release reports (<ID>-<VERSION_ID>); Distro and
// Codename select the third-party apt repositories.
type OS struct{ Name, Distro, Codename string }

// OSes is the one list of supported operating systems. Adding one needs
// nginx.org, packages.sury.org and the MariaDB mirror to publish for its
// codename.
var OSes = []OS{
	{"ubuntu-24.04", "ubuntu", "noble"},
	{"ubuntu-26.04", "ubuntu", "resolute"},
	{"debian-13", "debian", "trixie"},
}

// LookupOS returns the supported OS called name.
func LookupOS(name string) (OS, bool) {
	for _, o := range OSes {
		if o.Name == name {
			return o, true
		}
	}
	return OS{}, false
}

// SupportedOS returns the names of OSes.
func SupportedOS() []string {
	names := make([]string, len(OSes))
	for i, o := range OSes {
		names[i] = o.Name
	}
	return names
}
