package release_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/host/hosttest"
	"github.com/roots/trellis-cli/pkg/converge/release"
)

func spec() release.Spec {
	return release.Spec{
		Site: "example.com", ID: "20261125120000-abc1234", Artifact: "/var/lib/trellis/artifacts/abc.tar.gz",
		Shared:   []release.Link{{Path: ".env", Target: "shared/.env"}, {Path: "web/app/uploads", Target: "shared/uploads"}},
		PHPUnit:  "php8.4-fpm.service",
		UpdateDB: true,
		Hooks:    release.Hooks{Release: "echo release", After: "wp cache flush"},
		Env:      []string{"TRELLIS_ENV=production"},
	}
}

func names(steps []release.Step) []string {
	var out []string
	for _, s := range steps {
		out = append(out, s.Name)
	}
	return out
}

func TestStepsOrder(t *testing.T) {
	got := strings.Join(names(release.Steps(spec())), "\n")
	want := strings.Join([]string{
		"create /srv/www/example.com/releases/20261125120000-abc1234",
		"unpack",
		"link .env → shared/.env",
		"link web/app/uploads → shared/uploads",
		"check PHP platform",
		"release hook",
		"switch current → 20261125120000-abc1234",
		"update database",
		"clear theme roots cache",
		"reload php8.4-fpm.service",
		"after hook",
		"prune releases (keep 5)",
		"remove artifacts",
	}, "\n")
	if got != want {
		t.Errorf("steps:\n%s\nwant:\n%s", got, want)
	}
	for _, s := range release.Steps(spec()) {
		live := s.Name == "update database" || s.Name == "clear theme roots cache" || strings.HasPrefix(s.Name, "reload") || s.Name == "after hook" ||
			strings.HasPrefix(s.Name, "prune") || s.Name == "remove artifacts"
		if s.Live != live {
			t.Errorf("%s: live = %v", s.Name, s.Live)
		}
	}
}

func activate(t *testing.T, h *hosttest.Fake) error {
	t.Helper()
	return release.Activate(context.Background(), h, spec(), func(string, string) {})
}

func TestFailureBeforeSwitchLeavesSiteUnchanged(t *testing.T) {
	h := hosttest.New()
	h.Fail["php -r"] = "Composer detected issues in your platform: PHP >= 8.5 required"
	err := activate(t, h)
	if err == nil || !strings.Contains(err.Error(), "unchanged") || !strings.Contains(err.Error(), "PHP >= 8.5") {
		t.Fatalf("err = %v", err)
	}
	if len(h.Commands("/bin/sh -c ln -sfn")) != 0 {
		t.Error("switched despite the failed check")
	}
	if len(h.Commands("rm -rf /srv/www/example.com/releases/20261125120000-abc1234")) != 1 {
		t.Errorf("half-made release not removed: %v", h.Log)
	}
}

func TestOptionalFailuresDontFailTheDeploy(t *testing.T) {
	h := hosttest.New()
	h.Fail["/bin/sh -c wp core is-installed"] = "database error"
	if err := activate(t, h); err != nil {
		t.Fatal(err)
	}
	if len(h.Commands("sudo -n /usr/bin/systemctl reload php8.4-fpm.service")) != 1 {
		t.Error("php-fpm not reloaded")
	}
}

func TestFailureAfterSwitchSaysReleaseIsLive(t *testing.T) {
	h := hosttest.New()
	h.Fail["sudo -n /usr/bin/systemctl reload"] = "failed"
	if err := activate(t, h); err == nil || !strings.Contains(err.Error(), "is live") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateRejectsEscapes(t *testing.T) {
	for _, mut := range []func(*release.Spec){
		func(s *release.Spec) { s.Site = "../etc" },
		func(s *release.Spec) { s.ID = "a/b" },
		func(s *release.Spec) { s.Shared = []release.Link{{Path: "../x", Target: "shared/x"}} },
		func(s *release.Spec) { s.Shared = []release.Link{{Path: "x", Target: "/etc"}} },
		func(s *release.Spec) { s.Artifact = "relative.tar.gz" },
	} {
		s := spec()
		mut(&s)
		if s.Validate() == nil {
			t.Errorf("accepted %+v", s)
		}
	}
}

func TestRollback(t *testing.T) {
	h := hosttest.New()
	h.Outputs = map[string]string{
		"readlink /srv/www/example.com/current": "releases/20261125120000-bbb\n",
		"ls -1 /srv/www/example.com/releases":   "20261120000000-aaa\n20261125120000-bbb\n",
	}
	from, to, err := release.Rollback(context.Background(), h, "example.com", "php8.4-fpm.service", func(string, string) {})
	if err != nil || from != "20261125120000-bbb" || to != "20261120000000-aaa" {
		t.Fatalf("rollback %s → %s: %v", from, to, err)
	}
	if len(h.Commands("sudo -n /usr/bin/systemctl reload php8.4-fpm.service")) != 1 {
		t.Error("php-fpm not reloaded")
	}
	if _, err := release.Previous([]string{"a"}, "a"); err == nil {
		t.Error("rolled back past the first release")
	}
}

func TestExistingReleaseIsNotReused(t *testing.T) {
	h := hosttest.New()
	h.Fail["mkdir -m 0755"] = "mkdir: cannot create directory: File exists"
	if err := activate(t, h); err == nil || !strings.Contains(err.Error(), "File exists") {
		t.Fatalf("err = %v", err)
	}
	if len(h.Commands("tar")) != 0 {
		t.Error("unpacked into an existing release")
	}
	if len(h.Commands("rm -rf /srv/www/example.com/releases/")) != 0 {
		t.Error("deleted a release this deploy didn't create")
	}
}

func TestUpdateDBCanBeTurnedOff(t *testing.T) {
	s := spec()
	s.UpdateDB = false
	if slices.Contains(names(release.Steps(s)), "update database") {
		t.Error("update database ran with update_db: false")
	}
}
