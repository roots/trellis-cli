package compile_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/roots/trellis-cli/pkg/converge/compile"
	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/project"
)

// compileFiles compiles the minimal project's production environment with
// the given project files (and optional commands).
func compileFiles(t *testing.T, files fstest.MapFS, cmds ...config.Command) (*manifest.Manifest, error) {
	t.Helper()
	p, err := project.Open(filepath.Join("..", "testdata", "projects", "minimal"))
	if err != nil {
		t.Fatal(err)
	}
	e, err := p.Config.Resolve("production")
	if err != nil {
		t.Fatal(err)
	}
	e.Server.Commands = cmds
	sec := loadSecrets(t, p, "production")
	sec["basic_auth"] = map[string]any{"user": "u", "hash": "h"}
	return compile.Compile(compile.Input{Env: e, Lock: p.Lock, Secrets: sec, Project: files,
		Environments: []string{"production", "staging"}})
}

func file(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s), Mode: 0o644} }

func TestServerFileRules(t *testing.T) {
	unit := "[Service]\nExecStart=/bin/true\n[Install]\nWantedBy=multi-user.target\n"
	m, err := compileFiles(t, fstest.MapFS{
		"server/files/etc/apt/sources.list.d/tailscale.sources":       file("Types: deb\n"),
		"server/files/etc/nginx/conf.d/rate.conf":                     file("limit_req_zone x;\n"),
		"server/files/etc/nginx/includes.d/example.com/a.conf":        file("# a\n"),
		"server/files/etc/nginx/includes.d/elsewhere.com/a.conf":      file("# a site this env doesn't have\n"),
		"server/files/etc/nginx/htpasswd/example.com.tmpl":            file(`{{ secret "basic_auth/user" }}:{{ secret "basic_auth/hash" }}`),
		"server/files/etc/systemd/system/worker.service":              file(unit),
		"server/files/etc/systemd/system/job.service":                 file("[Service]\nType=oneshot\nExecStart=/bin/true\n"),
		"server/files/etc/systemd/system/job.timer":                   file("[Timer]\nOnCalendar=daily\n[Install]\nWantedBy=timers.target\n"),
		"server/files/etc/systemd/system/nginx.service.d/limits.conf": file("[Service]\nLimitNOFILE=65536\n"),
		"server/files/etc/containers/systemd/search.container":        file("[Container]\nImage=x\n" + "[Install]\nWantedBy=multi-user.target\n"),
		"server/files/etc/sudoers.d/deployers":                        file("web ALL=(root) NOPASSWD: /usr/bin/true\n"),
		"server/files/etc/sysctl.d/90-tuning.conf":                    file("vm.swappiness = 10\n"),
		"server/files/usr/local/bin/report":                           file("#!/bin/sh\n"),
		"server/files/opt/thing/config.json":                          {Data: []byte("{}"), Mode: 0o755},
		"server/files/.gitkeep":                                       file(""),
		"server/env/production/files/etc/motd":                        file("production\n"),
		"server/env/staging/files/etc/motd":                           file("staging\n"),
		"server/env/production/files/etc/nginx/conf.d/rate.conf":      file("# production wins\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	notifies := func(path, key string) bool {
		r := get(t, m, manifest.KindFile, path)
		return slices.ContainsFunc(r.Notify, func(n manifest.Notify) bool { return strings.Contains(n.Key(), key) })
	}

	if r := get(t, m, manifest.KindFile, "/etc/apt/sources.list.d/tailscale.sources"); r.Phase != manifest.PhaseRepos || !notifies(r.Name, "apt-get\x00update") {
		t.Errorf("apt sources: phase %v notify %v", r.Phase, r.Notify)
	}
	if !notifies("/etc/nginx/conf.d/rate.conf", "nginx.service") {
		t.Error("nginx files must reload nginx")
	}
	if c := get(t, m, manifest.KindFile, "/etc/nginx/conf.d/rate.conf").File.Content; c != "# production wins\n" {
		t.Errorf("the environment overlay must win: %q", c)
	}
	get(t, m, manifest.KindFile, "/etc/nginx/includes.d/example.com/a.conf")
	if m.Get(manifest.ID{Kind: manifest.KindFile, Name: "/etc/nginx/includes.d/elsewhere.com/a.conf"}) != nil {
		t.Error("includes for a site this environment doesn't have must be skipped")
	}
	if h := get(t, m, manifest.KindFile, "/etc/nginx/htpasswd/example.com").File; !h.Sensitive || h.Mode != 0o640 || h.Group != "www-data" || h.Content != "u:h\n" {
		t.Errorf("htpasswd = %+v", h)
	}

	// Units: [Install] means enabled and started.
	if s := get(t, m, manifest.KindService, "worker.service").Service; s.Enabled == nil || !*s.Enabled || !s.Active {
		t.Errorf("worker = %+v", s)
	}
	if !notifies("/etc/systemd/system/worker.service", "systemctl\x00daemon-reload") || !notifies("/etc/systemd/system/worker.service", "unit:worker.service") {
		t.Error("a unit change must daemon-reload and restart it")
	}
	if m.Get(manifest.ID{Kind: manifest.KindService, Name: "job.service"}) != nil {
		t.Error("a unit without [Install] is started by its trigger, not by Trellis")
	}
	if !notifies("/etc/systemd/system/job.service", "unit:job.service") {
		t.Error("a unit without [Install] must still restart (if running) when its file changes")
	}
	for path, unit := range map[string]string{
		"/etc/systemd/system/job.service":          "job.service",
		"/etc/systemd/system/worker.service":       "worker.service",
		"/etc/containers/systemd/search.container": "search.service",
	} {
		if u := get(t, m, manifest.KindFile, path).File.Unit; u != unit {
			t.Errorf("%s defines unit %q, want %q (stopped when the file is removed)", path, u, unit)
		}
	}
	get(t, m, manifest.KindService, "job.timer")
	if !notifies("/etc/systemd/system/nginx.service.d/limits.conf", "unit:nginx.service") {
		t.Error("a drop-in must restart its unit")
	}
	get(t, m, manifest.KindPackage, "podman")
	if s := get(t, m, manifest.KindService, "search.service").Service; s.Enabled != nil || !s.Active {
		t.Errorf("quadlet units are enabled by their generator: %+v", s)
	}

	if f := get(t, m, manifest.KindFile, "/etc/sudoers.d/deployers").File; f.Mode != 0o440 || len(f.Validate) == 0 {
		t.Errorf("sudoers = %+v", f)
	}
	if !notifies("/etc/sysctl.d/90-tuning.conf", "sysctl\x00--system") {
		t.Error("sysctl.d must apply")
	}
	if get(t, m, manifest.KindFile, "/usr/local/bin/report").File.Mode != 0o755 {
		t.Error("/usr/local/bin files are executable")
	}
	if f := get(t, m, manifest.KindFile, "/opt/thing/config.json"); f.File.Mode != 0o755 || len(f.Notify) != 0 {
		t.Errorf("unknown paths keep the project file's mode and have no effect: %+v", f)
	}
	if get(t, m, manifest.KindFile, "/etc/motd").File.Content != "production\n" {
		t.Error("only this environment's overlay applies")
	}
	if m.Get(manifest.ID{Kind: manifest.KindFile, Name: "/.gitkeep"}) != nil {
		t.Error("dotfiles are ignored")
	}
}

func TestServerFileErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		path, want string
	}{
		"trellis owns it":    {"server/files/etc/nginx/nginx.conf", "managed by Trellis"},
		"denied prefix":      {"server/files/srv/www/example.com/x", "can't be managed"},
		"other php version":  {"server/files/etc/php/8.1/fpm/conf.d/x.ini", "use /etc/php/8.4/"},
		"cron name with dot": {"server/files/etc/cron.d/a.cron", "cron ignores"},
		"old location":       {"server/nginx/http.d/x.conf", "moved to server/files/etc/nginx/conf.d/"},
		"unknown location":   {"server/stuff/x", "unknown location"},
		"unknown env":        {"server/env/prod/files/etc/x", `no environment named "prod"`},
		"bad template":       {"server/files/etc/x.tmpl", "template"},
		"missing secret":     {"server/files/etc/y.tmpl", "nope"},
	} {
		t.Run(name, func(t *testing.T) {
			content := "x\n"
			switch name {
			case "bad template":
				content = "{{ .Nope"
			case "missing secret":
				content = `{{ secret "nope" }}`
			}
			_, err := compileFiles(t, fstest.MapFS{tc.path: file(content)})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestCommands(t *testing.T) {
	fonts := func(font string) fstest.MapFS {
		return fstest.MapFS{"server/files/usr/local/share/fonts/a.ttf": file(font)}
	}
	cmd := config.Command{Run: "fc-cache -f", On: []string{"/usr/local/share/fonts/"}}
	inputs := func(fs fstest.MapFS) string {
		m, err := compileFiles(t, fs, cmd)
		if err != nil {
			t.Fatal(err)
		}
		return get(t, m, manifest.KindExec, "user:fc-cache-f").Exec.Inputs
	}
	if inputs(fonts("v1")) == inputs(fonts("v2")) {
		t.Error("a change to a watched file must change the command's inputs")
	}
	if a, b := inputs(fonts("v1")), inputs(fonts("v1")); a != b {
		t.Error("inputs must be stable")
	}

	// Watching a Trellis-managed file works too.
	if _, err := compileFiles(t, nil, config.Command{Run: "true", On: []string{"/etc/nginx/nginx.conf"}}); err != nil {
		t.Error(err)
	}
	if _, err := compileFiles(t, nil, cmd); err == nil || !strings.Contains(err.Error(), "matches no file") {
		t.Errorf("on: with nothing to watch must fail, got %v", err)
	}
	m, err := compileFiles(t, nil, config.Command{Name: "x", Run: "true", Env: map[string]string{"K": "secret:sites/example.com/db_password", "PLAIN": "v"}})
	if err != nil {
		t.Fatal(err)
	}
	x := get(t, m, manifest.KindExec, "user:x")
	if x.Exec.Env["PLAIN"] != "v" || x.Exec.Env["K"] == "" || strings.HasPrefix(x.Exec.Env["K"], "secret:") {
		t.Errorf("env = %v", x.Exec.Env)
	}
	if red := x.Redacted(); !manifest.IsRedacted(red.Exec.Env["K"]) || !manifest.IsRedacted(red.Exec.Env["PLAIN"]) {
		t.Errorf("env must be redacted in history: %v", red.Exec.Env)
	}
}
