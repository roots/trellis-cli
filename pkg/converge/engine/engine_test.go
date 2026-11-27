package engine_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/compile"
	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/engine"
	"github.com/roots/trellis-cli/pkg/converge/host/hosttest"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/project"
	"github.com/roots/trellis-cli/pkg/converge/state"
)

func newFake() *hosttest.Fake { return hosttest.New() }

// compileEnv compiles a test project environment, letting mutate edit the
// resolved environment first (to simulate trellis.yml changes).
func compileEnv(t testing.TB, proj, env string, mutate func(*config.Environment)) *manifest.Manifest {
	t.Helper()
	p, err := project.Open(filepath.Join("..", "testdata", "projects", proj))
	if err != nil {
		t.Fatal(err)
	}
	e, err := p.Config.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(e)
	}
	doc, _, err := p.LoadSecrets(env, nil)
	if err != nil {
		t.Fatal(err)
	}
	sec, err := doc.Map()
	if err != nil {
		t.Fatal(err)
	}
	ms, err := compile.Compile(compile.Input{Env: e, Lock: p.Lock, Secrets: sec, Project: osDirFS(p.Root)})
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func plan(t testing.TB, h *hosttest.Fake, m *manifest.Manifest) (*engine.Plan, *state.Observed) {
	t.Helper()
	obs, err := engine.Observe(context.Background(), h, m)
	if err != nil {
		t.Fatal(err)
	}
	return engine.Compute(m, obs), obs
}

func converge(t testing.TB, h *hosttest.Fake, m *manifest.Manifest) *engine.Plan {
	t.Helper()
	p, obs := plan(t, h, m)
	if _, err := engine.Apply(context.Background(), h, m, p, obs.Previous, engine.Options{}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return p
}

func describe(p *engine.Plan) string {
	var b strings.Builder
	for _, a := range p.Actions {
		b.WriteString(string(a.Op) + " " + a.ID().String() + " " + strings.Join(a.Reasons, ",") + "\n")
	}
	return b.String()
}

func TestConvergesAndSecondPlanIsEmpty(t *testing.T) {
	for _, tc := range []struct{ project, env string }{
		{"minimal", "production"},
		{"minimal", "development"},
		{"full", "production"},
		{"external-db", "production"},
	} {
		t.Run(tc.project+"/"+tc.env, func(t *testing.T) {
			{
				m := compileEnv(t, tc.project, tc.env, nil)
				h := newFake()
				first := converge(t, h, m)
				if first.Empty() {
					t.Fatal("first plan should not be empty")
				}
				second, _ := plan(t, h, m)
				if !second.Empty() {
					t.Fatalf("%s: not converged after one apply:\n%s", m.Node, describe(second))
				}
				if second.Generation != first.Generation+1 {
					t.Errorf("generation = %d, want %d", second.Generation, first.Generation+1)
				}
			}
		})
	}
}

func TestFirstRunOrdering(t *testing.T) {
	m := compileEnv(t, "full", "production", nil)
	h := newFake()
	p, obs := plan(t, h, m)
	h.ResetLog() // only look at what apply runs
	if _, err := engine.Apply(context.Background(), h, m, p, obs.Previous, engine.Options{}); err != nil {
		t.Fatal(err)
	}

	index := func(prefix string) int {
		return slices.IndexFunc(h.Log, func(c string) bool { return strings.HasPrefix(c, prefix) })
	}
	order := []string{
		"timedatectl",      // before apt starts services that cache the timezone
		"apt-get update",   // after sources are written
		"apt-get install",  // one transaction
		"systemd-sysusers", // users before directories
		"systemd-tmpfiles", // directories before files inside them
		"systemctl daemon-reload",
		"mariadb", // databases after the server is installed
		"systemctl start nginx.service",
		"certbot certonly", // after nginx is serving the ACME location
	}
	last := -1
	for _, step := range order {
		i := index(step)
		if i < 0 {
			t.Fatalf("missing %q in:\n%s", step, strings.Join(h.Log, "\n"))
		}
		if i < last {
			t.Errorf("%q ran out of order", step)
		}
		last = i
	}
	if n := len(h.Commands("apt-get install -y")); n != 1 {
		t.Errorf("want a single apt transaction, got %d", n)
	}
	// nginx was started fresh, so its reload notifications are skipped.
	if len(h.Commands("systemctl try-reload-or-restart nginx.service")) != 1 {
		t.Errorf("want exactly one nginx reload (after cert issuance), got: %v", h.Commands("systemctl try-"))
	}
	// Let's Encrypt material replaced the placeholder.
	if got := string(h.Files["/etc/nginx/ssl/example.com.crt"].Data); got != "LE CERT example.com" {
		t.Errorf("cert = %q", got)
	}
}

func TestNoopRunIsConstantCost(t *testing.T) {
	m := compileEnv(t, "full", "production", nil)
	h := newFake()
	converge(t, h, m)
	h.ResetLog()

	p, _ := plan(t, h, m)
	if !p.Empty() {
		t.Fatalf("not empty:\n%s", describe(p))
	}
	// One dpkg-query, one simulated upgrade, one systemctl show, two DB
	// queries: regardless of how many resources the manifest has.
	if len(h.Log) != 5 {
		t.Errorf("observe ran %d commands:\n%s", len(h.Log), strings.Join(h.Log, "\n"))
	}
}

// Waiting updates are reported, without making the plan non-empty.
func TestWaitingUpdatesAreReported(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	converge(t, h, m)
	h.Upgradable = []string{"nginx", "php8.4-fpm", "not-installed"}
	p, _ := plan(t, h, m)
	if !p.Empty() {
		t.Fatalf("not empty:\n%s", describe(p))
	}
	want := "updates are waiting for 2 packages (nginx, php8.4-fpm)"
	if !slices.ContainsFunc(p.Warnings, func(w string) bool { return strings.HasPrefix(w, want) }) {
		t.Errorf("warnings = %q, want one starting %q", p.Warnings, want)
	}
}

// Upgrade installs what unattended-upgrades is allowed to now, and reports
// what it wasn't allowed to.
func TestUpgrade(t *testing.T) {
	ctx := context.Background()
	h := newFake()
	if _, err := engine.Upgrade(ctx, h, nil); err == nil || !strings.Contains(err.Error(), "converge apply") {
		t.Fatalf("upgrading an unconverged server: err = %v", err)
	}
	m := compileEnv(t, "minimal", "production", nil)
	converge(t, h, m)

	h.ResetLog()
	res, err := engine.Upgrade(ctx, h, nil)
	if err != nil || len(res.Upgraded) > 0 || len(h.Commands("unattended-upgrade")) > 0 {
		t.Fatalf("nothing waiting: res = %+v, err = %v, ran %v", res, err, h.Commands("unattended-upgrade"))
	}

	h.Upgradable = []string{"nginx", "php8.4-fpm", "mariadb-server"}
	h.Held = []string{"mariadb-server"}
	res, err = engine.Upgrade(ctx, h, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(res.Upgraded, " "); got != "nginx php8.4-fpm" {
		t.Errorf("upgraded %q", got)
	}
	if got := strings.Join(res.Waiting, " "); got != "mariadb-server" {
		t.Errorf("waiting %q", got)
	}
	if p, _ := plan(t, h, m); !p.Empty() {
		t.Errorf("not converged after upgrading:\n%s", describe(p))
	}
}

func TestDriftIsRepairedAndNotifies(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	converge(t, h, m)

	h.Files["/etc/nginx/nginx.conf"].Data = []byte("hand edited")
	h.Files["/etc/sudoers.d/90-trellis"].Mode = 0o644
	h.Units["fail2ban.service"].Active = false
	delete(h.Packages, "git")
	h.ResetLog()

	p, _ := plan(t, h, m)
	got := describe(p)
	for _, want := range []string{
		"update file:/etc/nginx/nginx.conf content",
		"update file:/etc/sudoers.d/90-trellis mode 0644 → 0440",
		"update service:fail2ban.service start",
		"create package:git not installed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plan missing %q:\n%s", want, got)
		}
	}
	converge(t, h, m)
	if len(h.Commands("nginx -t")) != 1 || len(h.Commands("systemctl try-reload-or-restart nginx.service")) != 1 {
		t.Errorf("nginx should be checked then reloaded once: %v", h.Log)
	}
	if p2, _ := plan(t, h, m); !p2.Empty() {
		t.Errorf("not converged:\n%s", describe(p2))
	}
}

func TestFailedCheckRollsBackFiles(t *testing.T) {
	ms := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	converge(t, h, ms)
	good := string(h.Files["/etc/nginx/sites/example.com.conf"].Data)

	changed := compileEnv(t, "minimal", "production", func(e *config.Environment) {
		e.Sites[0].Cache = true
	})
	h.Fail["nginx -t"] = "nginx: [emerg] bad config"
	h.ResetLog()
	p, obs := plan(t, h, changed)
	_, err := engine.Apply(context.Background(), h, changed, p, obs.Previous, engine.Options{})
	if err == nil || !strings.Contains(err.Error(), "check `nginx -t` failed") {
		t.Fatalf("expected check failure, got %v", err)
	}
	if got := string(h.Files["/etc/nginx/sites/example.com.conf"].Data); got != good {
		t.Error("site config was not restored after failed check")
	}
	if len(h.Commands("systemctl try-reload-or-restart nginx.service")) != 0 {
		t.Error("nginx must not be reloaded after a failed check")
	}

	gen, err := engine.LoadGeneration(h)
	if err != nil || !gen.Failed {
		t.Fatalf("failed generation not recorded: %+v %v", gen, err)
	}

	// Once fixed, the same change is planned again and applies.
	delete(h.Fail, "nginx -t")
	p2, _ := plan(t, h, changed)
	if !strings.Contains(describe(p2), "update file:/etc/nginx/sites/example.com.conf") {
		t.Errorf("rolled back change not re-planned:\n%s", describe(p2))
	}
	converge(t, h, changed)
	if p3, _ := plan(t, h, changed); !p3.Empty() {
		t.Errorf("not converged:\n%s", describe(p3))
	}
}

func TestValidateFailureNeverTouchesLiveFile(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	h.Files["/etc/sudoers.d/90-trellis"] = &hosttest.File{Data: []byte("old"), Mode: 0o440, Owner: "root", Group: "root"}
	h.Fail["visudo"] = "parse error"
	p, obs := plan(t, h, m)
	if _, err := engine.Apply(context.Background(), h, m, p, obs.Previous, engine.Options{}); err == nil {
		t.Fatal("expected validation error")
	}
	if string(h.Files["/etc/sudoers.d/90-trellis"].Data) != "old" {
		t.Error("live sudoers file replaced despite failed validation")
	}
}

func TestRemovingASitePrunesItsResources(t *testing.T) {
	h := newFake()
	converge(t, h, compileEnv(t, "full", "production", nil))

	without := compileEnv(t, "full", "production", func(e *config.Environment) {
		e.Sites = e.Sites[:1] // drop shop.example.com
	})
	p, _ := plan(t, h, without)
	got := describe(p)
	for _, want := range []string{
		"prune service:wp-cron-shop-example-com.timer",
		"prune database_user:shop_example_com@localhost",
		"prune cert:shop.example.com",
		"prune file:/etc/nginx/sites/shop.example.com.conf",
		"prune file:/etc/php/8.4/fpm/pool.d/shop.example.com.conf",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plan missing %q:\n%s", want, got)
		}
	}
	// Data is never dropped automatically.
	if strings.Contains(got, "prune database:shop_example_com_production") {
		t.Error("databases must not be pruned")
	}
	if !slices.ContainsFunc(p.Warnings, func(w string) bool { return strings.Contains(w, "shop_example_com_production is no longer managed") }) {
		t.Errorf("expected database warning, got %v", p.Warnings)
	}
	// Services stop before their files are removed.
	if strings.Index(got, "prune service:wp-cron-shop") > strings.Index(got, "prune file:/etc/systemd/system/wp-cron-shop") {
		t.Error("prune order: services must stop before files are removed")
	}

	converge(t, h, without)
	if _, ok := h.Files["/etc/nginx/sites/shop.example.com.conf"]; ok {
		t.Error("site config not removed")
	}
	if _, ok := h.DBUsers["shop_example_com@localhost"]; ok {
		t.Error("db user not dropped")
	}
	if !h.Databases["shop_example_com_production"] {
		t.Error("database must be kept")
	}
	if p2, _ := plan(t, h, without); !p2.Empty() {
		t.Errorf("not converged:\n%s", describe(p2))
	}
}

func TestFailedRunKeepsOwnershipForPruning(t *testing.T) {
	h := newFake()
	m := compileEnv(t, "full", "production", nil)
	h.Fail["/bin/sh -c fc-cache"] = "boom" // the user exec runs last, after files
	p, obs := plan(t, h, m)
	if _, err := engine.Apply(context.Background(), h, m, p, obs.Previous, engine.Options{}); err == nil {
		t.Fatal("expected failure")
	}
	delete(h.Fail, "/bin/sh -c fc-cache")

	// Exec that failed must re-run; resources that succeeded must not.
	p2, _ := plan(t, h, m)
	got := describe(p2)
	if !strings.Contains(got, "run exec:user:fc-cache-f") {
		t.Errorf("failed exec not retried:\n%s", got)
	}
	if strings.Contains(got, "file:/etc/nginx/nginx.conf") {
		t.Errorf("completed resources should not be re-applied:\n%s", got)
	}

	// Mail's files were written before the failure; removing mail from
	// config must still prune them.
	without := compileEnv(t, "full", "production", func(e *config.Environment) {
		e.Server.Mail = nil
	})
	p3, _ := plan(t, h, without)
	if !strings.Contains(describe(p3), "prune file:/etc/msmtprc") {
		t.Errorf("resources from a failed run were not pruned:\n%s", describe(p3))
	}
}

func TestExecRerunsOnlyWhenInputsChange(t *testing.T) {
	h := newFake()
	m := compileEnv(t, "full", "production", nil)
	converge(t, h, m)
	if n := len(h.Commands("/bin/sh -c fc-cache")); n != 1 {
		t.Fatalf("exec ran %d times", n)
	}
	converge(t, h, m)
	if n := len(h.Commands("/bin/sh -c fc-cache")); n != 1 {
		t.Errorf("exec re-ran without input changes")
	}
	changed := compileEnv(t, "full", "production", func(e *config.Environment) {
		e.Server.Commands[0].Run = "fc-cache -fv"
	})
	converge(t, h, changed)
	if n := len(h.Commands("/bin/sh -c fc-cache -fv")); n != 1 {
		t.Errorf("exec did not re-run after its command changed")
	}
	// Secret env reaches the command but never history.
	if c := h.Commands("/bin/sh -c tailscale up"); len(c) != 1 || !slices.Contains(h.Envs[c[0]], "TS_AUTHKEY=tskey-auth-example") {
		t.Errorf("tailscale-up ran with %+v", c)
	}
}

func TestDownloadVerification(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	sury := "https://packages.sury.org/php/apt.gpg"
	for name, tamper := range map[string]func([]byte) []byte{
		// A different key than the pinned one (key material changed).
		"different key": func(k []byte) []byte { k = slices.Clone(k); k[20] ^= 0xff; return k },
		// The genuine key plus an extra one: apt would trust both.
		"extra key appended": func(k []byte) []byte {
			extra := slices.Clone(k)
			extra[20] ^= 0xff
			return append(slices.Clone(k), extra...)
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newFake()
			h.URLs[sury] = tamper(h.URLs[sury])
			p, obs := plan(t, h, m)
			_, err := engine.Apply(context.Background(), h, m, p, obs.Previous, engine.Options{})
			if err == nil || !strings.Contains(err.Error(), "which is not pinned") {
				t.Fatalf("expected fingerprint error, got %v", err)
			}
			if _, ok := h.Files["/etc/apt/keyrings/sury-php.gpg"]; ok {
				t.Error("unverified key was installed")
			}
		})
	}
}

func TestPasswordChangeUpdatesDBUser(t *testing.T) {
	h := newFake()
	converge(t, h, compileEnv(t, "minimal", "production", nil))

	m := compileEnv(t, "minimal", "production", nil)
	for i := range m.Resources {
		if r := &m.Resources[i]; r.Kind == manifest.KindDatabaseUser {
			u := *r.DatabaseUser
			u.Password = "rotated"
			r.DatabaseUser = &u
		}
	}
	p, _ := plan(t, h, m)
	if !strings.Contains(describe(p), "update database_user:example_com@localhost password or grants changed") {
		t.Fatalf("plan:\n%s", describe(p))
	}
	converge(t, h, m)
	if h.DBUsers["example_com@localhost"] != "rotated" {
		t.Error("password not rotated")
	}
}

func TestPlanHashBindsApproval(t *testing.T) {
	h := newFake()
	m := compileEnv(t, "minimal", "production", nil)
	a, _ := plan(t, h, m)
	b, _ := plan(t, h, m)
	if a.Hash() != b.Hash() {
		t.Error("same state must give the same plan hash")
	}
	h.Packages["git"] = "1"
	c, _ := plan(t, h, m)
	if a.Hash() == c.Hash() {
		t.Error("different state must give a different plan hash")
	}
}

// A fresh install defers daemon starts (policy-rc.d), so each daemon
// starts once, with Trellis's config, and needs no restart.
func TestFreshInstallStartsDaemonsOnceWithConfig(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	converge(t, h, m)
	for _, unit := range []string{"php8.4-fpm.service", "fail2ban.service"} {
		if len(h.Commands("systemctl start "+unit)) != 1 {
			t.Errorf("%s not started once", unit)
		}
		if n := len(h.Commands("systemctl try-reload-or-restart "+unit)) + len(h.Commands("systemctl try-restart "+unit)); n != 0 {
			t.Errorf("%s restarted after a fresh start", unit)
		}
		if c := h.Commands("systemctl enable --no-reload " + unit); len(c) != 0 {
			t.Errorf("%s was enabled by its package; enabling it again is redundant (and slow for SysV-backed units)", unit)
		}
	}
	if len(h.Commands("apt-get install -y -q -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold -o Dpkg::Options::=--force-unsafe-io")) != 1 {
		t.Errorf("fresh install without unsafe I/O: %v", h.Commands("apt-get install"))
	}
	if _, ok := h.Files["/usr/sbin/policy-rc.d"]; ok {
		t.Error("policy-rc.d left behind")
	}
	if len(h.Commands("fetch http://deb.example/")) == 0 {
		t.Error("packages weren't prefetched")
	}
}

// When an install also upgrades installed packages, daemon starts can't be
// deferred (policy-rc.d would block the upgrade's restarts too). Packages
// then start their services with their default config, so those must still
// be reloaded rather than treated as fresh starts.
func TestServicesStartedByPackagesAreReloaded(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	// An installed daemon that this install upgrades.
	h.Packages["openssh-server"], h.PackageUnits["openssh-server"], h.Upgradable = "0.9", "ssh.service", []string{"openssh-server"}
	converge(t, h, m)
	if got := h.Commands("systemctl start php8.4-fpm.service"); len(got) != 0 {
		t.Errorf("php-fpm was already running; start is a no-op: %v", got)
	}
	for _, unit := range []string{"php8.4-fpm.service", "fail2ban.service"} {
		n := len(h.Commands("systemctl try-reload-or-restart "+unit)) + len(h.Commands("systemctl try-restart "+unit))
		if n == 0 {
			t.Errorf("%s kept the package's default config:\n%s", unit, strings.Join(h.Commands("systemctl"), "\n"))
		}
	}
	if _, ok := h.Files["/usr/sbin/policy-rc.d"]; ok || len(h.Commands("dpkg-query -L -- openssh-server")) != 1 {
		t.Error("deferral must be ruled out by checking the upgraded package for daemons")
	}
}

// Fresh images always pull some library upgrades; those don't rule out
// deferring daemon starts. Unsafe I/O is only for a server's first
// converge.
func TestLibraryUpgradesStillDeferAndUnsafeIOOnlyFirst(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	h.Packages["libc6"], h.Upgradable = "0.9", []string{"libc6"}
	converge(t, h, m)
	if len(h.Commands("systemctl start php8.4-fpm.service")) != 1 {
		t.Error("a library upgrade must not prevent deferred starts")
	}
	if len(h.Commands("sync")) != 1 {
		t.Error("unsafe I/O must be followed by a sync")
	}

	// A later converge that installs a new package: safe I/O.
	h.Packages["libc6"] = "1.0-fake"
	with := manifest.New(m.Node, m.Roles)
	with.OS = m.OS
	for _, r := range append(slices.Clone(m.Resources), manifest.NewPackage("jq", "")) {
		if err := with.Add(r); err != nil {
			t.Fatal(err)
		}
	}
	with.Finalize()
	h.ResetLog()
	converge(t, h, with)
	if c := h.Commands("apt-get install -y"); len(c) != 1 || strings.Contains(c[0], "unsafe-io") {
		t.Errorf("only a first converge may use unsafe I/O: %v", c)
	}
}

// Units a package wanted started but Trellis doesn't manage (e.g. a
// user's tailscaled, PHP's session cleanup timer) still start, after
// Trellis's files are written.
func TestDeferredStartsOfUnmanagedUnits(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	h.PackageUnits["php8.4-cli"] = "phpsessionclean.timer"
	converge(t, h, m)
	if len(h.Commands("systemctl start phpsessionclean.timer")) != 1 || !h.Units["phpsessionclean.timer"].Active {
		t.Errorf("a package's own unit wasn't started: %v", h.Commands("systemctl start"))
	}
	if _, ok := h.Files["/run/trellis/policy-rc.d.log"]; ok {
		t.Error("the denied-starts log wasn't cleared")
	}
}

func TestStalePolicyIsRemovedAndForeignOneRespected(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)

	h := newFake()
	h.Files["/usr/sbin/policy-rc.d"] = &hosttest.File{Data: []byte("#!/bin/sh\n# Trellis: defers daemon starts during a package install.\nexit 101\n"), Mode: 0o755}
	converge(t, h, m)
	if _, ok := h.Files["/usr/sbin/policy-rc.d"]; ok {
		t.Error("a stale Trellis policy-rc.d must be removed")
	}

	h = newFake()
	h.Files["/usr/sbin/policy-rc.d"] = &hosttest.File{Data: []byte("#!/bin/sh\nexit 0\n"), Mode: 0o755}
	converge(t, h, m)
	if string(h.Files["/usr/sbin/policy-rc.d"].Data) != "#!/bin/sh\nexit 0\n" {
		t.Error("someone else's policy-rc.d was changed")
	}
	if len(h.Commands("systemctl start php8.4-fpm.service")) != 0 {
		t.Error("with a foreign policy-rc.d (allowing starts), packages start their own daemons")
	}
}

func TestParseDenied(t *testing.T) {
	log := "fail2ban start 5\nfail2ban.service start\nmsmtpd (start) 5\nmariadb stop 5\nphp8.4-fpm restart 5\nphpsessionclean.timer start\n"
	got := strings.Join(engine.ParseDenied([]byte(log)), " ")
	if want := "fail2ban.service msmtpd.service php8.4-fpm.service phpsessionclean.timer"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Deferring starts is only safe when apt's simulation is understood: an
// unknown format must not read as "no upgrades".
func TestParseSimulateFailsSafe(t *testing.T) {
	known := `NOTE: This is only a simulation!
The following NEW packages will be installed:
  nginx
Inst libc6 [2.39-0ubuntu8.3] (2.39-0ubuntu8.4 Ubuntu:24.04/noble-updates [arm64])
Inst nginx (1.29.3-1~noble nginx.org:noble [arm64])
Remv mlocate [1.1.18-1]
Conf libc6 (2.39-0ubuntu8.4 Ubuntu:24.04/noble-updates [arm64])
Conf nginx (1.29.3-1~noble nginx.org:noble [arm64])
`
	got, ok := engine.ParseSimulate([]byte(known))
	if !ok || strings.Join(got, " ") != "libc6 mlocate" {
		t.Errorf("got %q (ok %v), want libc6 mlocate", got, ok)
	}
	for _, out := range []string{
		"",
		"Conf nginx (1.29.3-1~noble nginx.org:noble [arm64])\n",
		"Inst nginx (1.29.3-1~noble [arm64])\nInst libc6 2.39-0ubuntu8.3 -> 2.39-0ubuntu8.4\n",
		"Install libc6 [2.39-0ubuntu8.3] (2.39-0ubuntu8.4)\n",
	} {
		if _, ok := engine.ParseSimulate([]byte(out)); ok {
			t.Errorf("%q: parsed as understood", out)
		}
	}
}

// A failed phase command must be retried by the next run, not forgotten
// because the files that trigger it were already written.
func TestFailedPhaseCommandIsRetried(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
	h.Fail["systemd-tmpfiles"] = "unsafe path transition"
	p, obs := plan(t, h, m)
	if _, err := engine.Apply(context.Background(), h, m, p, obs.Previous, engine.Options{}); err == nil {
		t.Fatal("expected failure")
	}
	delete(h.Fail, "systemd-tmpfiles")
	h.ResetLog()
	converge(t, h, m)
	if len(h.Commands("systemd-tmpfiles")) == 0 {
		t.Error("systemd-tmpfiles was not re-run after failing")
	}
	if p, _ := plan(t, h, m); !p.Empty() {
		t.Errorf("not converged:\n%s", describe(p))
	}
}

// Removing a unit file stops its service; the file's own restart
// notification must not then try to restart a unit that no longer exists.
func TestRemovingAUnitDoesNotRestartIt(t *testing.T) {
	h := newFake()
	m := compileEnv(t, "full", "production", nil)
	converge(t, h, m)

	without := manifest.New(m.Node, m.Roles)
	without.OS = m.OS
	for _, r := range m.Resources {
		if r.Name != "example-actions.service" && r.Name != "/etc/systemd/system/example-actions.service" {
			if err := without.Add(r); err != nil {
				t.Fatal(err)
			}
		}
	}
	without.Finalize()
	converge(t, h, without)
	if len(h.Commands("systemctl disable --now example-actions.service")) != 1 {
		t.Error("the unit was not stopped")
	}
	if c := h.Commands("systemctl try-restart example-actions.service"); len(c) != 0 {
		t.Errorf("a pruned unit was restarted: %v", c)
	}
}

func TestRemovingAUnitWithoutInstallStopsIt(t *testing.T) {
	h := newFake()
	m := compileEnv(t, "full", "production", nil)
	unit := manifest.NewRootFile("/etc/systemd/system/job.service", "[Service]\nExecStart=/bin/true\n", 0o644,
		manifest.RestartUnit("job.service")).DefiningUnit("job.service")
	with := manifest.New(m.Node, m.Roles)
	with.OS = m.OS
	for _, r := range append(slices.Clone(m.Resources), unit) {
		if err := with.Add(r); err != nil {
			t.Fatal(err)
		}
	}
	with.Finalize()
	converge(t, h, with)
	restarts := len(h.Commands("systemctl try-restart job.service"))
	converge(t, h, m)
	if len(h.Commands("systemctl stop job.service")) != 1 {
		t.Errorf("a removed unit without a service resource must be stopped: %v", h.Commands("systemctl"))
	}
	if c := h.Commands("systemctl try-restart job.service"); len(c) != restarts {
		t.Errorf("a pruned unit was restarted: %v", c)
	}
}

func TestEnablingUnitsReloadsSystemdOnce(t *testing.T) {
	h := newFake()
	converge(t, h, compileEnv(t, "full", "production", nil))
	cmds := h.Commands("systemctl")
	lastEnable, reloads := -1, 0
	for i, c := range cmds {
		switch {
		case strings.HasPrefix(c, "systemctl enable "):
			if !strings.HasPrefix(c, "systemctl enable --no-reload ") {
				t.Errorf("%s reloads systemd itself", c)
			}
			lastEnable = i
		case c == "systemctl daemon-reload" && lastEnable >= 0:
			reloads++
		}
	}
	if lastEnable < 0 || reloads != 1 {
		t.Errorf("want one daemon-reload after the enables, got %d:\n%s", reloads, strings.Join(cmds, "\n"))
	}
}
