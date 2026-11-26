// Package release activates a built release on a server. It runs inside
// trellis-agent, so it depends only on host and the standard library.
//
// A deploy is a fixed sequence of steps (Steps is a pure function of the
// Spec): unpack the uploaded artifact into releases/<id>, link shared
// paths, check the PHP platform, run the release hook, switch `current`
// atomically, then WordPress upkeep, a graceful PHP-FPM reload, the after
// hook and pruning. A failure before the switch removes the new release and
// leaves the site untouched.
package release

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/host"
)

// Root is where sites live.
const Root = "/srv/www"

// Spec describes one release of one site.
type Spec struct {
	Site     string `json:"site"`
	ID       string `json:"id"`       // releases/<ID>
	Artifact string `json:"artifact"` // uploaded .tar.gz on the server
	// Shared links paths in the release to paths under shared/.
	Shared    []Link   `json:"shared"`
	PHPUnit   string   `json:"php_unit"` // e.g. php8.4-fpm.service
	Multisite bool     `json:"multisite,omitempty"`
	UpdateDB  bool     `json:"update_db,omitempty"` // wp core update-db after the switch
	Hooks     Hooks    `json:"hooks"`
	Env       []string `json:"env,omitempty"` // for hooks: TRELLIS_ENV, …
	Keep      int      `json:"keep"`          // releases to keep
}

// Link is a release-relative path and the shared/-relative path it points to.
type Link struct {
	Path   string `json:"path"`
	Target string `json:"target"`
}

// Hooks are shell scripts run as the web user from the release directory.
type Hooks struct {
	Release string `json:"release,omitempty"` // before the switch; failure aborts
	After   string `json:"after,omitempty"`   // after the switch
}

// Step is one command of a deploy.
type Step struct {
	Name string
	Cmd  host.Cmd
	// Live steps run after the switch: the release is already serving, so
	// a failure no longer rolls back.
	Live bool
	// Optional failures are reported but don't fail the deploy.
	Optional bool
}

func (s Spec) siteDir() string    { return path.Join(Root, s.Site) }
func (s Spec) releaseDir() string { return path.Join(s.siteDir(), "releases", s.ID) }

// Validate rejects specs that could escape the site directory.
func (s Spec) Validate() error {
	for _, v := range []string{s.Site, s.ID} {
		if v == "" || strings.ContainsAny(v, "/\\") || v == "." || v == ".." {
			return fmt.Errorf("invalid site or release id %q", v)
		}
	}
	for _, l := range s.Shared {
		for _, p := range []string{l.Path, l.Target} {
			if p == "" || path.IsAbs(p) || strings.HasPrefix(path.Clean(p), "..") {
				return fmt.Errorf("invalid shared path %q", p)
			}
		}
	}
	if !path.IsAbs(s.Artifact) {
		return fmt.Errorf("invalid artifact path %q", s.Artifact)
	}
	return nil
}

func asWeb(dir string, env []string, argv ...string) host.Cmd {
	return host.Cmd{Argv: append([]string{"runuser", "-u", "web", "--"}, argv...), Dir: dir, Env: env}
}

// Steps is the deploy sequence for s.
func Steps(s Spec) []Step {
	site, rel := s.siteDir(), s.releaseDir()
	current := path.Join(site, "current")
	env := append([]string{"TRELLIS_SITE=" + s.Site, "TRELLIS_RELEASE=" + rel, "TRELLIS_CURRENT=" + current}, s.Env...)
	steps := []Step{
		// mkdir (not install -d) so an existing release is an error, not reused.
		{Name: "create " + rel, Cmd: host.Cmd{Argv: []string{"/bin/sh", "-c", `mkdir -m 0755 "$1" && chown web:www-data "$1"`, "sh", rel}}},
		{Name: "unpack", Cmd: asWeb(rel, nil, "tar", "-xzf", s.Artifact, "-C", rel)},
	}
	for _, l := range s.Shared {
		link, target := path.Join(rel, l.Path), path.Join(site, l.Target)
		steps = append(steps, Step{Name: "link " + l.Path + " → " + l.Target,
			Cmd: asWeb(rel, nil, "/bin/sh", "-c", `rm -rf "$1" && mkdir -p "$(dirname "$1")" && ln -s "$2" "$1"`, "sh", link, target)})
	}
	// Composer's platform check: the build ran without platform checks
	// (the laptop's PHP isn't the server's), so verify against this PHP.
	steps = append(steps, Step{Name: "check PHP platform",
		Cmd: asWeb(rel, nil, "php", "-r", `if (is_file("vendor/composer/platform_check.php")) { require "vendor/composer/platform_check.php"; }`)})
	if s.Hooks.Release != "" {
		steps = append(steps, Step{Name: "release hook", Cmd: asWeb(rel, env, "/bin/sh", "-ec", s.Hooks.Release)})
	}
	steps = append(steps, Step{Name: "switch current → " + s.ID,
		Cmd: asWeb(site, nil, "/bin/sh", "-c", `ln -sfn "releases/$1" current.new && mv -T current.new current`, "sh", s.ID)})

	network := ""
	if s.Multisite {
		network = " --network"
	}
	transient := "wp option delete _site_transient_theme_roots"
	if s.Multisite {
		transient = "wp network meta delete 1 _site_transient_theme_roots"
	}
	if s.UpdateDB {
		steps = append(steps, Step{Name: "update database", Live: true, Optional: true,
			Cmd: asWeb(current, nil, "/bin/sh", "-c", "wp core is-installed"+network+" 2>/dev/null || exit 0; wp core update-db"+network)})
	}
	steps = append(steps,
		// Theme paths are cached with the old release's absolute path.
		Step{Name: "clear theme roots cache", Live: true, Optional: true,
			Cmd: asWeb(current, nil, "/bin/sh", "-c", "wp core is-installed"+network+" 2>/dev/null || exit 0; "+transient+" >/dev/null 2>&1 || true")},
		Step{Name: "reload " + s.PHPUnit, Live: true, Cmd: host.Cmd{Argv: []string{"systemctl", "reload", s.PHPUnit}}},
	)
	if s.Hooks.After != "" {
		steps = append(steps, Step{Name: "after hook", Live: true, Optional: true, Cmd: asWeb(current, env, "/bin/sh", "-ec", s.Hooks.After)})
	}
	keep := s.Keep
	if keep < 1 {
		keep = 5
	}
	steps = append(steps,
		Step{Name: "prune releases (keep " + strconv.Itoa(keep) + ")", Live: true, Optional: true,
			Cmd: asWeb(path.Join(site, "releases"), nil, "/bin/sh", "-c", `ls -1 | sort -r | tail -n +$(($1 + 1)) | xargs -r rm -rf --`, "sh", strconv.Itoa(keep))},
		Step{Name: "remove artifact", Live: true, Optional: true, Cmd: host.Cmd{Argv: []string{"rm", "-f", s.Artifact}}},
	)
	return steps
}

// Activate runs the deploy. log receives each step and its outcome.
func Activate(ctx context.Context, h host.Host, s Spec, log func(step, detail string)) error {
	if err := s.Validate(); err != nil {
		return err
	}
	var warnings []error
	created := false // only ever remove a release this run created
	for i, st := range Steps(s) {
		log(st.Name, "")
		out, err := h.Run(ctx, st.Cmd)
		if err == nil {
			created = created || i == 0
			continue
		}
		err = fmt.Errorf("%s: %w%s", st.Name, err, trimOutput(out))
		switch {
		case st.Optional:
			log(st.Name, "warning: "+err.Error())
			warnings = append(warnings, err)
		case !st.Live:
			// Not live yet: remove the half-made release; the site is untouched.
			if created {
				_, _ = h.Run(ctx, host.Cmd{Argv: []string{"rm", "-rf", s.releaseDir()}})
			}
			return fmt.Errorf("%w (the site is unchanged)", err)
		default:
			return fmt.Errorf("%w (release %s is live)", err, s.ID)
		}
	}
	if len(warnings) > 0 {
		log("done", fmt.Sprintf("%d warning(s)", len(warnings)))
	}
	return nil
}

func trimOutput(out []byte) string {
	if s := strings.TrimSpace(string(out)); s != "" {
		return ": " + s
	}
	return ""
}

// Previous returns the release before current in releases (sorted by name).
func Previous(releases []string, current string) (string, error) {
	rs := slices.Clone(releases)
	slices.Sort(rs)
	i := slices.Index(rs, current)
	switch {
	case i < 0:
		return "", fmt.Errorf("current release %q not found in releases/", current)
	case i == 0:
		return "", errors.New("no earlier release to roll back to")
	}
	return rs[i-1], nil
}

// Rollback switches site's current release to the previous one and
// reloads PHP-FPM. It returns the (from, to) releases.
func Rollback(ctx context.Context, h host.Host, site, phpUnit string, log func(step, detail string)) (from, to string, err error) {
	s := Spec{Site: site, ID: "x", Artifact: "/x"}
	if err := s.Validate(); err != nil {
		return "", "", err
	}
	dir := s.siteDir()
	out, err := h.Run(ctx, host.Cmd{Argv: []string{"readlink", path.Join(dir, "current")}})
	if err != nil {
		return "", "", fmt.Errorf("%s has no current release", site)
	}
	from = path.Base(strings.TrimSpace(string(out)))
	out, err = h.Run(ctx, host.Cmd{Argv: []string{"ls", "-1", path.Join(dir, "releases")}})
	if err != nil {
		return "", "", err
	}
	if to, err = Previous(strings.Fields(string(out)), from); err != nil {
		return from, "", err
	}
	for _, st := range []Step{
		{Name: "switch current → " + to, Cmd: asWeb(dir, nil, "/bin/sh", "-c", `ln -sfn "releases/$1" current.new && mv -T current.new current`, "sh", to)},
		{Name: "reload " + phpUnit, Cmd: host.Cmd{Argv: []string{"systemctl", "reload", phpUnit}}},
	} {
		log(st.Name, "")
		if out, err := h.Run(ctx, st.Cmd); err != nil {
			return from, to, fmt.Errorf("%s: %w%s", st.Name, err, trimOutput(out))
		}
	}
	return from, to, nil
}
