// Package hosttest provides Fake, an in-memory host.Host that simulates the
// slice of Ubuntu the engine drives (files, dpkg/apt, systemctl, MariaDB,
// gpg, certbot). It lets the whole observe → plan → apply loop run in unit
// tests, including convergence ("a second plan is empty"), rollback and
// pruning, without a VM.
package hosttest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/roots/trellis-cli/pkg/converge/host"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/state"
)

type File struct {
	Data  []byte
	Mode  fs.FileMode
	Owner string
	Group string
}

type Unit struct {
	Enabled bool
	Active  bool
}

type Fake struct {
	mu sync.Mutex

	Files     map[string]*File
	Packages  map[string]string // installed name → version
	Units     map[string]*Unit
	Databases map[string]bool
	DBUsers   map[string]string // "user@host" → password
	// URLs served by Fetch.
	URLs map[string][]byte
	// GPG maps key content to the fingerprint gpg reports for it.
	GPG map[string]string
	// Fail makes commands whose argv (joined by spaces) has this prefix
	// fail with the given message.
	Fail map[string]string
	// DBDown makes all mariadb commands fail (server not installed yet).
	DBDown bool
	// PackageFiles are files a package creates when installed (like
	// nginx's conf.d/default.conf), to simulate package side effects.
	PackageFiles map[string][]string
	// PackageUnits are units a package's postinst enables and starts,
	// with whatever config exists at that moment.
	PackageUnits map[string]string

	// Outputs are canned stdout for commands with these prefixes.
	Outputs map[string]string

	// Log is every command run, joined by spaces.
	Log []string
}

func New() *Fake {
	return &Fake{
		Files: map[string]*File{
			"/etc/os-release": {Data: []byte("PRETTY_NAME=\"Ubuntu 24.04.3 LTS\"\nID=ubuntu\nVERSION_ID=\"24.04\"\nVERSION_CODENAME=noble\n"), Mode: 0o644, Owner: "root", Group: "root"},
		},
		Packages:  map[string]string{},
		Units:     map[string]*Unit{},
		Databases: map[string]bool{"information_schema": true, "mysql": true},
		DBUsers:   map[string]string{"root@localhost": ""},
		URLs:      map[string][]byte{},
		PackageFiles: map[string][]string{
			"nginx":      {"/etc/nginx/conf.d/default.conf"},
			"php8.4-fpm": {"/etc/php/8.4/fpm/pool.d/www.conf"},
			"php8.3-fpm": {"/etc/php/8.3/fpm/pool.d/www.conf"},
		},
		PackageUnits: map[string]string{
			"php8.4-fpm":     "php8.4-fpm.service",
			"php8.3-fpm":     "php8.3-fpm.service",
			"mariadb-server": "mariadb.service",
			"fail2ban":       "fail2ban.service",
			"redis-server":   "redis-server.service",
		},
		GPG:  map[string]string{},
		Fail: map[string]string{},
	}
}

var _ host.Host = (*Fake)(nil)

func (f *Fake) Stat(path string, hash bool) (state.FileInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, ok := f.Files[path]
	if !ok {
		return state.FileInfo{}, nil
	}
	fi := state.FileInfo{Exists: true, Mode: file.Mode, Owner: file.Owner, Group: file.Group}
	if hash {
		sum := sha256.Sum256(file.Data)
		fi.SHA256 = hex.EncodeToString(sum[:])
	}
	return fi, nil
}

func (f *Fake) ReadFile(path string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	file, ok := f.Files[path]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	return slices.Clone(file.Data), nil
}

func (f *Fake) WriteFile(path string, data []byte, mode fs.FileMode, owner, group string, validate func(string) error) error {
	if !strings.HasPrefix(path, state.Dir+"/") && strings.Contains(string(data), manifest.RedactedPrefix) {
		return fmt.Errorf("write %s: content is a redacted secret", path)
	}
	if validate != nil {
		staged := path + ".trellis-staged"
		f.put(staged, data, mode, owner, group)
		err := validate(staged)
		f.mu.Lock()
		delete(f.Files, staged)
		f.mu.Unlock()
		if err != nil {
			return err
		}
	}
	f.put(path, data, mode, owner, group)
	return nil
}

func (f *Fake) put(path string, data []byte, mode fs.FileMode, owner, group string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Files[path] = &File{Data: slices.Clone(data), Mode: mode, Owner: owner, Group: group}
}

func (f *Fake) Remove(path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Files, path)
	return nil
}

func (f *Fake) Fetch(_ context.Context, url string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = append(f.Log, "fetch "+url)
	if b, ok := f.URLs[url]; ok {
		return slices.Clone(b), nil
	}
	return []byte("content of " + url), nil
}

// Commands returns logged commands with the given prefix.
func (f *Fake) Commands(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.Log {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// ResetLog clears the command log.
func (f *Fake) ResetLog() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = nil
}

func (f *Fake) Run(_ context.Context, c host.Cmd) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	line := strings.Join(c.Argv, " ")
	f.Log = append(f.Log, line)
	// Redacted secrets come from stored history; using one as a real
	// value (a password, file content) is always a bug.
	if strings.Contains(line, manifest.RedactedPrefix) || strings.Contains(string(c.Stdin), manifest.RedactedPrefix) ||
		strings.Contains(strings.Join(c.Env, " "), manifest.RedactedPrefix) {
		return nil, fmt.Errorf("%s: command uses a redacted secret", line)
	}
	for prefix, msg := range f.Fail {
		if strings.HasPrefix(line, prefix) {
			return nil, fmt.Errorf("%s: %s", line, msg)
		}
	}
	for prefix, out := range f.Outputs {
		if strings.HasPrefix(line, prefix) {
			return []byte(out), nil
		}
	}
	argv := c.Argv
	if argv[0] == "/bin/sh" && len(argv) == 3 && strings.Contains(argv[2], "gpg --homedir") {
		argv = []string{"gpg"} // fingerprint check in a throwaway GnuPG home
	}
	switch argv[0] {
	case "dpkg-query":
		return f.dpkgQuery(argv), nil
	case "apt-get":
		return nil, f.apt(argv)
	case "systemctl":
		return f.systemctl(argv)
	case "mariadb":
		return f.mariadb(string(c.Stdin))
	case "gpg":
		if fpr, ok := f.GPG[string(c.Stdin)]; ok {
			return []byte("pub:-:255:22:ABC:::::::\nfpr:::::::::" + fpr + ":\n"), nil
		}
		return []byte("pub:-:255:22:ABC:::::::\nfpr:::::::::0000000000000000000000000000000000000000:\n"), nil
	case "certbot":
		return nil, f.certbot(argv)
	}
	// Checks, sysusers, tmpfiles, visudo, timedatectl, sh: succeed.
	return nil, nil
}

func (f *Fake) dpkgQuery(argv []string) []byte {
	i := slices.Index(argv, "--")
	var b strings.Builder
	for _, name := range argv[i+1:] {
		if v, ok := f.Packages[name]; ok {
			fmt.Fprintf(&b, "%s\t%s\tii \n", name, v)
		}
	}
	return []byte(b.String())
}

func (f *Fake) apt(argv []string) error {
	if len(argv) < 2 {
		return nil
	}
	i := slices.Index(argv, "--")
	switch argv[1] {
	case "install":
		for _, spec := range argv[i+1:] {
			name, version, ok := strings.Cut(spec, "=")
			if !ok {
				version = "1.0-fake"
			}
			if _, installed := f.Packages[name]; !installed {
				for _, p := range f.PackageFiles[name] {
					f.Files[p] = &File{Data: []byte("from package " + name), Mode: 0o644, Owner: "root", Group: "root"}
				}
				if unit, ok := f.PackageUnits[name]; ok {
					u := f.unit(unit)
					u.Enabled, u.Active = true, true
				}
			}
			f.Packages[name] = version
		}
	case "remove":
		for _, name := range argv[i+1:] {
			delete(f.Packages, name)
		}
	}
	return nil
}

func (f *Fake) unit(name string) *Unit {
	u, ok := f.Units[name]
	if !ok {
		u = &Unit{}
		f.Units[name] = u
	}
	return u
}

func (f *Fake) systemctl(argv []string) ([]byte, error) {
	switch argv[1] {
	case "show":
		i := slices.Index(argv, "--")
		var b strings.Builder
		for n, name := range argv[i+1:] {
			if n > 0 {
				b.WriteString("\n")
			}
			u, ok := f.Units[name]
			load, active, file := "not-found", "inactive", ""
			if ok {
				load, file = "loaded", "disabled"
				if u.Enabled {
					file = "enabled"
				}
				if u.Active {
					active = "active"
				}
			}
			fmt.Fprintf(&b, "Id=%s\nLoadState=%s\nActiveState=%s\nUnitFileState=%s\n", name, load, active, file)
		}
		return []byte(b.String()), nil
	case "enable":
		f.unit(argv[2]).Enabled = true
	case "disable":
		unit := argv[len(argv)-1]
		f.unit(unit).Enabled = false
		if slices.Contains(argv, "--now") {
			f.unit(unit).Active = false
		}
	case "start":
		f.unit(argv[2]).Active = true
	case "stop":
		f.unit(argv[2]).Active = false
	}
	return nil, nil
}

var (
	createDBRe   = regexp.MustCompile("CREATE DATABASE IF NOT EXISTS `([^`]+)`")
	createUserRe = regexp.MustCompile(`CREATE USER IF NOT EXISTS '([^']*)'@'([^']*)' IDENTIFIED BY '([^']*)'`)
	alterUserRe  = regexp.MustCompile(`ALTER USER '([^']*)'@'([^']*)' IDENTIFIED BY '([^']*)'`)
	dropUserRe   = regexp.MustCompile(`DROP USER IF EXISTS '([^']*)'@'([^']*)'`)
)

func (f *Fake) mariadb(sql string) ([]byte, error) {
	if f.DBDown {
		return nil, fmt.Errorf("mariadb: can't connect to local server through socket")
	}
	switch {
	case strings.Contains(sql, "information_schema.SCHEMATA"):
		return []byte(strings.Join(slices.Sorted(maps.Keys(f.Databases)), "\n") + "\n"), nil
	case strings.Contains(sql, "FROM mysql.user"):
		return []byte(strings.Join(slices.Sorted(maps.Keys(f.DBUsers)), "\n") + "\n"), nil
	}
	for _, m := range createDBRe.FindAllStringSubmatch(sql, -1) {
		f.Databases[m[1]] = true
	}
	for _, m := range createUserRe.FindAllStringSubmatch(sql, -1) {
		if _, ok := f.DBUsers[m[1]+"@"+m[2]]; !ok {
			f.DBUsers[m[1]+"@"+m[2]] = m[3]
		}
	}
	for _, m := range alterUserRe.FindAllStringSubmatch(sql, -1) {
		f.DBUsers[m[1]+"@"+m[2]] = m[3]
	}
	for _, m := range dropUserRe.FindAllStringSubmatch(sql, -1) {
		delete(f.DBUsers, m[1]+"@"+m[2])
	}
	return nil, nil
}

func (f *Fake) certbot(argv []string) error {
	if argv[1] != "certonly" {
		return nil
	}
	name := argv[slices.Index(argv, "--cert-name")+1]
	live := "/etc/letsencrypt/live/" + name
	f.Files[live+"/fullchain.pem"] = &File{Data: []byte("LE CERT " + name), Mode: 0o644, Owner: "root", Group: "root"}
	f.Files[live+"/privkey.pem"] = &File{Data: []byte("LE KEY " + name), Mode: 0o600, Owner: "root", Group: "root"}
	return nil
}
