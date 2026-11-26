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

const suryFingerprint = "15058500A0235D97F5D10063B188E2B695BD4743"

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
	if n := len(h.Commands("apt-get install")); n != 1 {
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
	// One dpkg-query, one systemctl show, two DB queries: regardless of
	// how many resources the manifest has.
	if len(h.Log) != 4 {
		t.Errorf("observe ran %d commands:\n%s", len(h.Log), strings.Join(h.Log, "\n"))
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
		e.Sites[0].Cache = config.CacheFastCGI
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
	if !strings.Contains(got, "run exec:user:rebuild-font-cache") {
		t.Errorf("failed exec not retried:\n%s", got)
	}
	if strings.Contains(got, "file:/etc/nginx/nginx.conf") {
		t.Errorf("completed resources should not be re-applied:\n%s", got)
	}

	// Meilisearch's files were written before the failure; removing the
	// service from config must still prune them.
	without := compileEnv(t, "full", "production", func(e *config.Environment) {
		e.Server.Services = nil
	})
	p3, _ := plan(t, h, without)
	if !strings.Contains(describe(p3), "prune file:/etc/containers/systemd/meilisearch.container") {
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
		e.Server.Exec[0].Run = "fc-cache -fv"
	})
	converge(t, h, changed)
	if n := len(h.Commands("/bin/sh -c fc-cache -fv")); n != 1 {
		t.Errorf("exec did not re-run after its command changed")
	}
}

func TestDownloadVerification(t *testing.T) {
	h := newFake()
	// A different (valid) key than the pinned one: the Sury key with one
	// byte of the key material changed.
	key := slices.Clone(h.URLs["https://packages.sury.org/php/apt.gpg"])
	key[20] ^= 0xff
	h.URLs["https://packages.sury.org/php/apt.gpg"] = key
	m := compileEnv(t, "minimal", "production", nil)
	p, obs := plan(t, h, m)
	_, err := engine.Apply(context.Background(), h, m, p, obs.Previous, engine.Options{})
	if err == nil || !strings.Contains(err.Error(), "does not contain key "+suryFingerprint) {
		t.Fatalf("expected fingerprint error, got %v", err)
	}
	if _, ok := h.Files["/etc/apt/keyrings/sury-php.gpg"]; ok {
		t.Error("unverified key was installed")
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

// Debian packages start their services on install, before Trellis writes
// their config. Those must still be reloaded, not treated as fresh starts.
func TestServicesStartedByPackagesAreReloaded(t *testing.T) {
	m := compileEnv(t, "minimal", "production", nil)
	h := newFake()
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
