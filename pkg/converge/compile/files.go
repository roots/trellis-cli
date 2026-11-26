package compile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"text/template"

	"github.com/roots/trellis-cli/pkg/converge/config"
	m "github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
	"github.com/roots/trellis-cli/pkg/converge/templates"
)

// User customization is files and commands (see CUSTOMIZING.md):
//
//   - server/files/<path> is installed at /<path> on every environment's
//     server, and server/env/<env>/files/<path> on that environment's only
//     (overriding server/files/ for the same path). Files ending in .tmpl
//     are rendered, with access to config and secrets.
//   - A file's phase and effect (reload nginx, daemon-reload, enable a unit,
//     apt-get update, ...) come from its path: see fileRule.
//   - commands: run last, and re-run when a file they watch changes.

// deniedPrefixes can't be managed with server/files/: they're virtual,
// volatile, or owned by deploys and Trellis itself.
var deniedPrefixes = []string{
	"/proc/", "/sys/", "/dev/", "/run/", "/tmp/", "/boot/",
	"/srv/www/", "/var/lib/trellis/", "/usr/local/lib/trellis/",
}

// movedLocations are the old fixed drop-in directories under server/.
var movedLocations = map[string]string{
	"nginx/http.d":   "server/files/etc/nginx/conf.d/",
	"nginx/sites":    "server/files/etc/nginx/includes.d/<site>/",
	"php/conf.d":     "server/files/etc/php/<version>/fpm/conf.d/",
	"mariadb/conf.d": "server/files/etc/mysql/mariadb.conf.d/",
	"systemd":        "server/files/etc/systemd/system/<unit>.d/",
}

var (
	installSectionRe = regexp.MustCompile(`(?m)^\s*\[Install\]\s*$`)
	envFileRe        = regexp.MustCompile(`(?m)^\s*EnvironmentFile=-?(/\S+)\s*$`)
	unitSuffixes     = []string{".service", ".timer", ".socket", ".path", ".target", ".mount", ".slice"}
	// Units that can be restarted when their file changes. Restarting a
	// target, mount or slice would stop what depends on it.
	restartableSuffixes = []string{".service", ".timer", ".socket", ".path"}
	quadletSuffixes     = []string{".container", ".volume", ".network", ".pod", ".kube", ".image", ".build"}
	sysctlApply         = m.RunCommand("sysctl", "--system")
)

// serverFiles compiles server/files/ and this environment's overlay. It
// runs after everything Trellis compiles itself, so a path Trellis already
// manages is reported as such.
func (b *builder) serverFiles() {
	if b.in.Project == nil {
		return
	}
	b.checkServerDir()
	files := map[string]string{} // server path → project path
	b.collectFiles("server/files", files)
	b.collectFiles("server/env/"+b.env.Name+"/files", files)
	started := map[string]string{} // unit → its file's content
	for _, dest := range slices.Sorted(maps.Keys(files)) {
		b.serverFile(dest, files[dest], started)
	}
	b.restartOnEnvironmentFiles(started)
}

// restartOnEnvironmentFiles makes a unit's EnvironmentFile= restart it
// when the file changes (e.g. a rotated secret), if Trellis manages it.
func (b *builder) restartOnEnvironmentFiles(started map[string]string) {
	for _, unit := range slices.Sorted(maps.Keys(started)) {
		for _, match := range envFileRe.FindAllStringSubmatch(started[unit], -1) {
			f := b.m.Get(m.ID{Kind: m.KindFile, Name: match[1]})
			if f == nil {
				continue
			}
			restart := m.RestartUnit(unit)
			if !slices.ContainsFunc(f.Notify, func(n m.Notify) bool { return n.Key() == restart.Key() }) {
				f.Notify = append(f.Notify, restart)
			}
		}
	}
}

// checkServerDir rejects unknown entries under server/, so a typo or an
// old drop-in location doesn't silently do nothing.
func (b *builder) checkServerDir() {
	entries, err := fs.ReadDir(b.in.Project, "server")
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case strings.HasPrefix(name, "."), name == "files", name == "ejected":
		case name == "env":
			b.checkEnvDir()
		default:
			moved := false
			for _, old := range slices.Sorted(maps.Keys(movedLocations)) {
				if (old == name || strings.HasPrefix(old, name+"/")) && exists(b.in.Project, "server/"+old) {
					b.errorf("server/%s/: moved to %s", old, movedLocations[old])
					moved = true
				}
			}
			if !moved {
				b.errorf("server/%s: unknown location (server/ holds files/, env/<environment>/files/ and ejected/)", name)
			}
		}
	}
}

func exists(fsys fs.FS, p string) bool {
	_, err := fs.Stat(fsys, p)
	return err == nil
}

func (b *builder) checkEnvDir() {
	envs, _ := fs.ReadDir(b.in.Project, "server/env")
	for _, e := range envs {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if len(b.in.Environments) > 0 && !slices.Contains(b.in.Environments, name) {
			b.errorf("server/env/%s: no environment named %q in trellis.yml", name, name)
			continue
		}
		inner, _ := fs.ReadDir(b.in.Project, "server/env/"+name)
		for _, f := range inner {
			if f.Name() != "files" && !strings.HasPrefix(f.Name(), ".") {
				b.errorf("server/env/%s/%s: unknown location (server/env/%s/ holds files/)", name, f.Name(), name)
			}
		}
	}
}

// collectFiles maps every file under root to its server path.
func (b *builder) collectFiles(root string, out map[string]string) {
	seen := map[string]string{}
	err := fs.WalkDir(b.in.Project, root, func(p string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) && p == root {
			return fs.SkipAll
		}
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil // .gitkeep, .DS_Store
		}
		if d.IsDir() {
			return nil
		}
		dest := "/" + strings.TrimSuffix(strings.TrimPrefix(p, root+"/"), ".tmpl")
		if other, ok := seen[dest]; ok {
			b.errorf("%s and %s are both installed at %s", other, p, dest)
			return nil
		}
		seen[dest] = p
		out[dest] = p
		return nil
	})
	if err != nil {
		b.errorf("%s: %v", root, err)
	}
}

// rule is what a path implies: when the file is installed and what a
// change to it does.
type rule struct {
	phase    m.Phase
	mode     fs.FileMode // 0: from the project file (0755 if executable, else 0644)
	group    string      // group for sensitive files that a service reads
	notify   []m.Notify
	validate []string
	service  *m.Resource // a unit to enable and start
	pkgs     []string    // packages the file needs (e.g. podman)
	unit     string      // the systemd unit the file defines
	site     *config.Site
	skip     bool // not for this environment (e.g. a site it doesn't have)
}

// fileRule is Trellis's table of path → phase and effect. Paths it doesn't
// know are installed with no effect; a command with on: can react to them.
func (b *builder) fileRule(dest, content string) (rule, error) {
	r := rule{phase: m.PhaseFiles}
	dir, name := path.Split(dest)
	under := func(prefix string) bool { return strings.HasPrefix(dest, prefix) }
	in := func(d string) bool { return dir == d }

	switch {
	case in("/etc/apt/keyrings/"), in("/usr/share/keyrings/"), in("/etc/apt/trusted.gpg.d/"),
		in("/etc/apt/sources.list.d/"), in("/etc/apt/preferences.d/"), in("/etc/apt/apt.conf.d/"):
		// Before packages are installed, so a repository's packages can be
		// listed in packages:.
		r.phase, r.notify = m.PhaseRepos, []m.Notify{aptUpdate}

	case under("/etc/nginx/"):
		r.notify, r.group = []m.Notify{nginxReload}, "www-data"
		if rest, ok := strings.CutPrefix(dest, "/etc/nginx/includes.d/"); ok {
			site, _, _ := strings.Cut(rest, "/")
			if r.site = b.findSite(site); r.site == nil {
				r.skip = true // a site this environment doesn't have
			}
		}

	case under("/etc/php/"):
		version, rest, _ := strings.Cut(strings.TrimPrefix(dest, "/etc/php/"), "/")
		if version != b.phpVersion() {
			return r, fmt.Errorf("PHP is %s on this server; use /etc/php/%s/", b.phpVersion(), b.phpVersion())
		}
		if strings.HasPrefix(rest, "fpm/") || strings.HasPrefix(rest, "mods-available/") {
			r.notify = []m.Notify{b.phpReload()}
		}

	case under("/etc/mysql/"):
		r.group = "mysql"
		if b.env.LocalDB() {
			r.notify = []m.Notify{mariadbRestart}
		}

	case in("/etc/ssh/sshd_config.d/"):
		r.notify = []m.Notify{sshReload}

	case in("/etc/sudoers.d/"):
		if strings.Contains(name, ".") || strings.HasSuffix(name, "~") {
			return r, errors.New("sudo ignores files in /etc/sudoers.d/ whose names contain . or end in ~")
		}
		r.mode, r.validate = 0o440, []string{"visudo", "-cf", "%s"}

	case in("/etc/cron.d/"):
		if strings.ContainsAny(name, ".") {
			return r, errors.New("cron ignores files in /etc/cron.d/ whose names contain a dot")
		}
		r.mode = 0o644

	case in("/etc/sysctl.d/"):
		r.notify = []m.Notify{sysctlApply}

	case in("/etc/systemd/journald.conf.d/"):
		r.notify = []m.Notify{m.RestartUnit("systemd-journald.service")}
	case in("/etc/systemd/timesyncd.conf.d/"):
		r.notify = []m.Notify{m.RestartUnit("systemd-timesyncd.service")}
	case in("/etc/systemd/resolved.conf.d/"):
		r.notify = []m.Notify{m.RestartUnit("systemd-resolved.service")}

	case under("/etc/systemd/system/") && strings.HasSuffix(path.Dir(dest), ".d") && path.Dir(path.Dir(dest)) == "/etc/systemd/system":
		// A drop-in for any unit, ours or a package's.
		unit := strings.TrimSuffix(path.Base(path.Dir(dest)), ".d")
		r.notify = []m.Notify{daemonReload}
		if !strings.Contains(unit, "@.") {
			r.notify = append(r.notify, m.RestartUnit(unit))
		}

	case in("/etc/systemd/system/") && hasSuffix(name, unitSuffixes):
		// A running unit restarts when its file changes and stops when the
		// file is removed. With [Install] it's also enabled and started;
		// without, whatever triggers it (a timer, a .path, a socket) starts it.
		r.notify = []m.Notify{daemonReload}
		if !strings.Contains(name, "@.") {
			if hasSuffix(name, restartableSuffixes) {
				r.unit = name
				r.notify = append(r.notify, m.RestartUnit(name))
			}
			if installSectionRe.MatchString(content) {
				svc := m.NewService(name, enabled, true)
				r.service = &svc
			}
		}

	case in("/etc/containers/systemd/") && hasSuffix(name, quadletSuffixes):
		// Podman Quadlet: the generator turns <name>.container into
		// <name>.service, enabled by its [Install] section.
		r.notify, r.pkgs = []m.Notify{daemonReload}, []string{"podman"}
		if strings.HasSuffix(name, ".container") {
			r.unit = strings.TrimSuffix(name, ".container") + ".service"
			r.notify = append(r.notify, m.RestartUnit(r.unit))
			if installSectionRe.MatchString(content) {
				svc := m.NewService(r.unit, nil, true)
				r.service = &svc
			}
		}

	case under("/etc/fail2ban/"):
		r.notify = []m.Notify{fail2banReload}

	case in("/usr/local/bin/"), in("/usr/local/sbin/"):
		r.mode = 0o755
	}
	return r, nil
}

func hasSuffix(s string, suffixes []string) bool {
	return slices.ContainsFunc(suffixes, func(x string) bool { return strings.HasSuffix(s, x) })
}

// serverFile compiles one file from server/files/ (or an overlay).
func (b *builder) serverFile(dest, src string, started map[string]string) {
	fail := func(err error) { b.errorf("%s: %v", src, err) }
	if path.Clean(dest) != dest {
		fail(fmt.Errorf("invalid server path %s", dest))
		return
	}
	for _, p := range deniedPrefixes {
		if strings.HasPrefix(dest, p) {
			fail(fmt.Errorf("%s can't be managed with server/files/", p))
			return
		}
	}
	for _, kind := range []m.Kind{m.KindFile, m.KindDownload} {
		if b.m.Get(m.ID{Kind: kind, Name: dest}) != nil {
			fail(fmt.Errorf("%s is managed by Trellis; add a file to a drop-in directory instead, or eject its template (trellis converge eject --list)", dest))
			return
		}
	}
	data, err := fs.ReadFile(b.in.Project, src)
	if err != nil {
		fail(err)
		return
	}
	info, err := fs.Stat(b.in.Project, src)
	if err != nil {
		fail(err)
		return
	}
	content := string(data)

	// The rule is decided on the rendered content ([Install] may come from
	// a template), but rendering needs the rule's site: render with the
	// path's site first.
	r, err := b.fileRule(dest, content)
	if err != nil {
		fail(err)
		return
	}
	if r.skip {
		return
	}
	sensitive := false
	if strings.HasSuffix(src, ".tmpl") {
		content, sensitive, err = b.renderUserFile(src, content, r.site)
		if err != nil {
			b.errs = append(b.errs, err)
			return
		}
		if r, err = b.fileRule(dest, content); err != nil {
			fail(err)
			return
		}
	}

	mode := r.mode
	if mode == 0 {
		mode = 0o644
		if info.Mode()&0o111 != 0 {
			mode = 0o755
		}
	}
	group := "root"
	if sensitive && r.mode == 0 {
		// Keep secrets from other users; services that read the file as
		// their own user get it through their group.
		mode = 0o600
		if r.group != "" {
			mode, group = 0o640, r.group
		}
	}
	file := m.NewFile(dest, content, mode, "root", group, r.notify...).InPhase(r.phase)
	if sensitive {
		file = file.AsSensitive()
	}
	if len(r.validate) > 0 {
		file = file.WithValidate(r.validate...)
	}
	if r.unit != "" {
		file = file.DefiningUnit(r.unit)
	}
	b.pkg(r.pkgs...)
	b.add(file)
	if r.service != nil && b.m.Get(r.service.ID()) == nil {
		b.add(*r.service)
		started[r.service.Name] = content
	}
}

// renderUserFile renders a .tmpl from server/files/. Templates see the
// environment, the server settings, every site (and .Site for files under
// /etc/nginx/includes.d/<site>/), and secrets via {{ secret "path" }}. A
// file that uses a secret is sensitive.
func (b *builder) renderUserFile(src, content string, site *config.Site) (string, bool, error) {
	sensitive := false
	funcs := template.FuncMap{"secret": func(p string) (string, error) {
		sensitive = true
		v, err := secrets.Resolve(b.in.Secrets, "secret:"+p)
		if err != nil {
			return "", fmt.Errorf("secret %q: %w", p, err)
		}
		return v, nil
	}}
	t, err := templates.ParseFuncs(src, content, funcs)
	if err != nil {
		return "", false, err
	}
	out, err := templates.Execute(t, map[string]any{
		"Env": b.env.Name, "Host": b.env.Host, "Server": b.env.Server, "Sites": b.env.Sites, "Site": site,
	})
	return out, sensitive, err
}

// commands compiles commands:. Each re-runs when its run, env or the
// digest of any file it watches changes. Watched files are resources in
// this manifest, so they're on the server before the command runs.
func (b *builder) commands() {
	for _, c := range b.env.Server.Commands {
		name := c.CommandName()
		h := sha256.New()
		for _, on := range c.On {
			watched := b.watched(on)
			if len(watched) == 0 {
				b.errorf("commands %s: on: %s matches no file Trellis manages (ship it under server/files/)", name, on)
			}
			for _, r := range watched {
				fmt.Fprintf(h, "%s\x00%s\n", r.Name, r.Digest())
			}
		}
		inputs := ""
		if len(c.On) > 0 {
			inputs = hex.EncodeToString(h.Sum(nil))
		}
		env := map[string]string{}
		for _, k := range slices.Sorted(maps.Keys(c.Env)) {
			env[k] = b.secret(c.Env[k], "commands."+name+".env."+k)
		}
		b.add(m.NewExec("user:"+name, []string{"/bin/sh", "-c", c.Run}, inputs).WithEnv(env))
	}
}

// watched returns the files and downloads at p, or under it if p is a
// directory.
func (b *builder) watched(p string) []m.Resource {
	dir := strings.TrimSuffix(p, "/") + "/"
	var out []m.Resource
	for _, r := range b.m.Resources {
		if r.Kind != m.KindFile && r.Kind != m.KindDownload {
			continue
		}
		if r.Name == p || strings.HasPrefix(r.Name, dir) {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, c m.Resource) int { return strings.Compare(a.Name, c.Name) })
	return out
}

func (b *builder) findSite(name string) *config.Site {
	for i := range b.env.Sites {
		if b.env.Sites[i].Name == name {
			return &b.env.Sites[i]
		}
	}
	return nil
}
