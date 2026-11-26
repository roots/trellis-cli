package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/roots/trellis-cli/pkg/converge/host"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/state"
)

// Event reports progress during apply.
type Event struct {
	Op      string // an action Op, or "run", "notify", "rollback", "warn"
	Target  string
	Message string
}

// Options configures Apply.
type Options struct {
	Log func(Event)
	Now func() time.Time
}

// Result summarises an apply.
type Result struct {
	Generation int
	Changed    int
	Notified   []string
}

// Apply executes plan p on h, converging it to desired. prev is the
// generation the plan was computed against.
//
// Execution order:
//  1. per phase: actions (packages in one apt transaction), then the
//     phase's command notifications (apt-get update, daemon-reload, ...);
//  2. prunes, in reverse phase order, with their command notifications;
//  3. deferred unit reloads/restarts, each gated on its check. A failed
//     check restores every file changed in this run that notifies the
//     unit, so a bad nginx config never replaces a good one;
//  4. Let's Encrypt issuance (needs nginx serving the new config);
//  5. the new generation is recorded. A failed run records what did
//     complete, and keeps everything attempted as owned for pruning.
func Apply(ctx context.Context, h host.Host, desired *manifest.Manifest, p *Plan, prev *state.Generation, opts Options) (*Result, error) {
	if opts.Log == nil {
		opts.Log = func(Event) {}
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	r := &run{
		ctx: ctx, h: h, opts: opts, desired: desired,
		backups:   map[string]backup{},
		completed: map[manifest.ID]bool{},
		started:   map[string]bool{},
		stopped:   map[string]bool{},
		triggers:  map[string][]string{},
		deferred:  map[string]manifest.Notify{},
	}
	for i := range desired.Resources {
		r.completed[desired.Resources[i].ID()] = true
	}
	for _, a := range p.Actions {
		if a.Op != OpPrune {
			r.completed[a.ID()] = false
		}
	}

	err := r.execute(p)
	if err := r.record(desired, prev, p.Generation, err != nil); err != nil {
		return nil, fmt.Errorf("record generation: %w", err)
	}
	if err != nil {
		return nil, err
	}
	return &Result{Generation: p.Generation, Changed: len(p.Actions), Notified: r.notified}, nil
}

type backup struct {
	info state.FileInfo
	data []byte
}

type run struct {
	ctx     context.Context
	h       host.Host
	opts    Options
	desired *manifest.Manifest
	// installed is set once packages were installed in this run.
	installed bool

	backups   map[string]backup          // original content of files changed in this run
	completed map[manifest.ID]bool       // desired resources known to be converged
	started   map[string]bool            // units started in this run
	stopped   map[string]bool            // units stopped by pruning in this run
	triggers  map[string][]string        // notify key → paths whose change triggered it
	deferred  map[string]manifest.Notify // unit → strongest pending notification
	order     []string                   // deferred units in first-seen order
	issue     []manifest.Resource        // Let's Encrypt certs to issue
	notified  []string
}

func (r *run) log(op, target, msg string) { r.opts.Log(Event{Op: op, Target: target, Message: msg}) }

func (r *run) execute(p *Plan) error {
	byPhase := map[manifest.Phase][]Action{}
	var prunes []Action
	for _, a := range p.Actions {
		if a.Op == OpPrune {
			prunes = append(prunes, a)
		} else {
			byPhase[a.Resource.Phase] = append(byPhase[a.Resource.Phase], a)
		}
	}
	for _, phase := range manifest.Phases() {
		acts := byPhase[phase]
		if phase == manifest.PhaseFiles && r.installed {
			extra, err := r.recheckAbsent(acts)
			if err != nil {
				return err
			}
			acts = append(acts, extra...)
		}
		if phase == manifest.PhaseServices && r.installed {
			var err error
			if acts, err = r.recheckStarted(acts); err != nil {
				return err
			}
		}
		if len(acts) == 0 {
			continue
		}
		if err := r.phase(acts); err != nil {
			return err
		}
	}
	if err := r.prune(prunes); err != nil {
		return err
	}
	if err := r.flushDeferred(); err != nil {
		return err
	}
	return r.issueCerts()
}

// recheckStarted drops "start" for units a package started earlier in
// this run (Debian postinst scripts start their services). Those units are
// running the package's default config, so they must not count as freshly
// started: their reload/restart notifications still have to run.
func (r *run) recheckStarted(acts []Action) ([]Action, error) {
	var units []string
	for _, a := range acts {
		if a.Resource.Kind == manifest.KindService && slices.Contains(a.Reasons, "start") {
			units = append(units, a.Resource.Service.Unit)
		}
	}
	if len(units) == 0 {
		return acts, nil
	}
	out, err := r.h.Run(r.ctx, systemctlShowCmd(units))
	if err != nil {
		return nil, err
	}
	live := parseSystemctlShow(out)
	kept := acts[:0:0]
	for _, a := range acts {
		if a.Resource.Kind == manifest.KindService {
			if u, ok := live[a.Resource.Service.Unit]; ok && u.Active() {
				a.Reasons = slices.DeleteFunc(slices.Clone(a.Reasons), func(s string) bool { return s == "start" })
				if len(a.Reasons) == 0 {
					r.completed[a.ID()] = true
					continue
				}
			}
		}
		kept = append(kept, a)
	}
	return kept, nil
}

// phase applies one phase's actions and then its command notifications.
func (r *run) phase(acts []Action) error {
	var pkgs []Action
	for _, a := range acts {
		if a.Resource.Kind == manifest.KindPackage {
			pkgs = append(pkgs, a)
			continue
		}
		if err := r.apply(a); err != nil {
			return fmt.Errorf("%s: %w", a.ID(), err)
		}
		r.completed[a.ID()] = true
	}
	if len(pkgs) > 0 {
		if err := r.installPackages(pkgs); err != nil {
			return err
		}
	}
	return r.notify(acts)
}

func (r *run) installPackages(acts []Action) error {
	pkgs := make([]manifest.Package, len(acts))
	names := make([]string, len(acts))
	for i, a := range acts {
		pkgs[i] = *a.Resource.Package
		names[i] = a.Resource.Name
	}
	r.log("install", strings.Join(names, " "), "")
	if _, err := r.h.Run(r.ctx, aptInstallCmd(pkgs)); err != nil {
		return err
	}
	r.installed = true
	for _, a := range acts {
		r.completed[a.ID()] = true
	}
	return nil
}

// recheckAbsent finds desired-absent files that a package install just
// created (e.g. nginx's conf.d/default.conf). They didn't exist when the
// plan was computed, so they aren't in it, but must go in this same run.
func (r *run) recheckAbsent(planned []Action) ([]Action, error) {
	inPlan := map[manifest.ID]bool{}
	for _, a := range planned {
		inPlan[a.ID()] = true
	}
	var extra []Action
	for _, res := range r.desired.Resources {
		if res.Kind != manifest.KindFile || !res.File.Absent || inPlan[res.ID()] {
			continue
		}
		fi, err := r.h.Stat(res.File.Path, false)
		if err != nil {
			return nil, err
		}
		if fi.Exists {
			extra = append(extra, Action{Op: OpRemove, Resource: res, Reasons: []string{"created by package install"}})
		}
	}
	return extra, nil
}

// notify runs command notifications of the given actions (deduplicated,
// in first-seen order) and queues unit notifications for the end.
func (r *run) notify(acts []Action) error {
	var cmds []manifest.Notify
	seen := map[string]bool{}
	for _, a := range acts {
		for _, n := range a.Resource.Notify {
			key := n.Key()
			if a.Resource.Kind == manifest.KindFile {
				r.triggers[key] = append(r.triggers[key], a.Resource.File.Path)
			}
			if n.Unit != "" {
				r.queue(n)
				continue
			}
			if !seen[key] {
				seen[key] = true
				cmds = append(cmds, n)
			}
		}
	}
	for _, n := range cmds {
		if err := r.check(n); err != nil {
			return err
		}
		r.log("run", strings.Join(n.Command, " "), "")
		if _, err := r.h.Run(r.ctx, host.Cmd{Argv: n.Command, Env: aptEnv}); err != nil {
			// Restore the files that triggered it, so the next run sees
			// them as changed, rewrites them and runs the command again.
			return errors.Join(err, r.rollback(n.Key()))
		}
	}
	return nil
}

// queue records a deferred unit notification; restart wins over reload.
func (r *run) queue(n manifest.Notify) {
	cur, ok := r.deferred[n.Unit]
	if !ok {
		r.order = append(r.order, n.Unit)
		r.deferred[n.Unit] = n
		return
	}
	if n.Action == manifest.Restart {
		cur.Action = manifest.Restart
	}
	if len(cur.Check) == 0 {
		cur.Check = n.Check
	}
	r.deferred[n.Unit] = cur
}

// check runs n's check; on failure it restores the files that triggered n.
func (r *run) check(n manifest.Notify) error {
	if len(n.Check) == 0 {
		return nil
	}
	if _, err := r.h.Run(r.ctx, host.Cmd{Argv: n.Check}); err != nil {
		rbErr := r.rollback(n.Key())
		return errors.Join(fmt.Errorf("check `%s` failed; restored %d file(s): %w",
			strings.Join(n.Check, " "), len(r.triggers[n.Key()]), err), rbErr)
	}
	return nil
}

func (r *run) rollback(key string) error {
	var errs []error
	for _, path := range r.triggers[key] {
		b, ok := r.backups[path]
		if !ok {
			continue
		}
		r.log("rollback", path, "")
		if b.info.Exists {
			errs = append(errs, r.h.WriteFile(path, b.data, b.info.Mode, b.info.Owner, b.info.Group, nil))
		} else {
			errs = append(errs, r.h.Remove(path))
		}
		r.completed[manifest.ID{Kind: manifest.KindFile, Name: path}] = false
	}
	return errors.Join(errs...)
}

func (r *run) flushDeferred() error {
	for _, unit := range r.order {
		n := r.deferred[unit]
		if r.started[unit] {
			continue // a fresh start already loaded the new config
		}
		if r.stopped[unit] {
			continue // pruned: its own unit file's removal notified it
		}
		if err := r.check(n); err != nil {
			return err
		}
		verb := "try-reload-or-restart"
		if n.Action == manifest.Restart {
			verb = "try-restart"
		}
		r.log("notify", unit, string(n.Action))
		if _, err := r.h.Run(r.ctx, host.Cmd{Argv: []string{"systemctl", verb, unit}}); err != nil {
			return err
		}
		r.notified = append(r.notified, string(n.Action)+" "+unit)
	}
	r.order, r.deferred = nil, map[string]manifest.Notify{}
	return nil
}

// backupFile snapshots path before the first change in this run.
func (r *run) backupFile(path string) error {
	if _, ok := r.backups[path]; ok {
		return nil
	}
	fi, err := r.h.Stat(path, false)
	if err != nil {
		return err
	}
	b := backup{info: fi}
	if fi.Exists && !fi.IsDir {
		if b.data, err = r.h.ReadFile(path); err != nil {
			return err
		}
	}
	r.backups[path] = b
	return nil
}

func (r *run) apply(a Action) error {
	res := &a.Resource
	r.log(string(a.Op), res.Describe(), strings.Join(a.Reasons, ", "))
	switch res.Kind {
	case manifest.KindFile:
		f := res.File
		if err := r.backupFile(f.Path); err != nil {
			return err
		}
		if f.Absent {
			return r.h.Remove(f.Path)
		}
		var validate func(string) error
		if len(f.Validate) > 0 {
			validate = func(staged string) error {
				argv := make([]string, len(f.Validate))
				for i, s := range f.Validate {
					argv[i] = strings.ReplaceAll(s, "%s", staged)
				}
				_, err := r.h.Run(r.ctx, host.Cmd{Argv: argv})
				return err
			}
		}
		return r.h.WriteFile(f.Path, []byte(f.Content), f.Mode, f.Owner, f.Group, validate)

	case manifest.KindDownload:
		return r.download(res.Download)

	case manifest.KindService:
		s := res.Service
		for _, reason := range a.Reasons {
			if _, err := r.h.Run(r.ctx, host.Cmd{Argv: []string{"systemctl", reason, s.Unit}}); err != nil {
				return err
			}
			if reason == "start" {
				r.started[s.Unit] = true
			}
		}
		return nil

	case manifest.KindDatabase:
		_, err := r.h.Run(r.ctx, mariadbCmd(res.Database.Server, createDatabaseSQL(res.Database)))
		return err

	case manifest.KindDatabaseUser:
		_, err := r.h.Run(r.ctx, mariadbCmd(res.DatabaseUser.Server, userSQL(res.DatabaseUser)))
		return err

	case manifest.KindCert:
		return r.cert(a)

	case manifest.KindExec:
		cmd := host.Cmd{Argv: res.Exec.Command}
		for _, k := range slices.Sorted(maps.Keys(res.Exec.Env)) {
			cmd.Env = append(cmd.Env, k+"="+res.Exec.Env[k])
		}
		if res.Exec.Stdin != "" {
			cmd.Stdin = []byte(res.Exec.Stdin)
		}
		_, err := r.h.Run(r.ctx, cmd)
		return err
	}
	return fmt.Errorf("unsupported kind %s", res.Kind)
}

func (r *run) download(d *manifest.Download) error {
	data, err := r.h.Fetch(r.ctx, d.URL)
	if err != nil {
		return err
	}
	if d.SHA256 != "" {
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); !strings.EqualFold(got, d.SHA256) {
			return fmt.Errorf("sha256 mismatch for %s: got %s, want %s", d.URL, got, d.SHA256)
		}
	}
	if len(d.Fingerprints) > 0 {
		fprs, err := keyFingerprints(data)
		if err != nil {
			return fmt.Errorf("%s: %w", d.URL, err)
		}
		// apt trusts every key in a keyring: each one must be pinned.
		for _, f := range fprs {
			if !slices.ContainsFunc(d.Fingerprints, func(p string) bool { return strings.EqualFold(p, f) }) {
				return fmt.Errorf("%s contains key %s, which is not pinned (pinned: %s)", d.URL, f, strings.Join(d.Fingerprints, ", "))
			}
		}
	}
	return r.h.WriteFile(d.Path, data, d.Mode, "root", "root", nil)
}

// cert installs a self-signed certificate (the final one for self-signed
// sites, a placeholder so nginx can start for Let's Encrypt sites) and
// queues Let's Encrypt issuance.
func (r *run) cert(a Action) error {
	c := a.Resource.Cert
	certInfo, err := r.h.Stat(c.CertPath, false)
	if err != nil {
		return err
	}
	if c.Provider == manifest.CertSelfSigned || !certInfo.Exists {
		certPEM, keyPEM, err := selfSigned(c.Domains, r.opts.Now())
		if err != nil {
			return err
		}
		if err := r.h.WriteFile(c.KeyPath, keyPEM, 0o600, "root", "root", nil); err != nil {
			return err
		}
		if err := r.h.WriteFile(c.CertPath, certPEM, 0o644, "root", "root", nil); err != nil {
			return err
		}
	}
	if c.Provider == manifest.CertLetsEncrypt {
		r.issue = append(r.issue, a.Resource)
	}
	return nil
}

func (r *run) issueCerts() error {
	if len(r.issue) == 0 {
		return nil
	}
	for _, res := range r.issue {
		c := res.Cert
		r.log("issue", res.Name, strings.Join(c.Domains, " "))
		if _, err := r.h.Run(r.ctx, certbotCmd(res.Name, c)); err != nil {
			r.completed[res.ID()] = false
			return fmt.Errorf("%s: %w", res.ID(), err)
		}
		live := "/etc/letsencrypt/live/" + res.Name
		for _, f := range []struct {
			src, dst string
			mode     fs.FileMode
		}{{live + "/privkey.pem", c.KeyPath, 0o600}, {live + "/fullchain.pem", c.CertPath, 0o644}} {
			data, err := r.h.ReadFile(f.src)
			if err != nil {
				return err
			}
			if err := r.h.WriteFile(f.dst, data, f.mode, "root", "root", nil); err != nil {
				return err
			}
		}
		for _, n := range res.Notify {
			if n.Unit != "" {
				// The unit must pick up the new certificate even if it was
				// started earlier in this run (with the placeholder).
				delete(r.started, n.Unit)
				r.queue(n)
			}
		}
	}
	r.issue = nil
	return r.flushDeferred()
}

// prune removes resources Trellis owned that are no longer desired.
func (r *run) prune(acts []Action) error {
	if len(acts) == 0 {
		return nil
	}
	var phases []manifest.Phase
	byPhase := map[manifest.Phase][]Action{}
	for _, a := range acts {
		if _, ok := byPhase[a.Resource.Phase]; !ok {
			phases = append(phases, a.Resource.Phase)
		}
		byPhase[a.Resource.Phase] = append(byPhase[a.Resource.Phase], a)
	}
	for _, phase := range phases {
		group := byPhase[phase]
		var pkgs []string
		for _, a := range group {
			if a.Resource.Kind == manifest.KindPackage {
				pkgs = append(pkgs, a.Resource.Name)
				continue
			}
			r.log("prune", a.Resource.Describe(), "")
			if err := r.pruneOne(a.Resource); err != nil {
				return fmt.Errorf("prune %s: %w", a.ID(), err)
			}
		}
		if len(pkgs) > 0 {
			r.log("prune", strings.Join(pkgs, " "), "")
			if _, err := r.h.Run(r.ctx, aptRemoveCmd(pkgs)); err != nil {
				return err
			}
		}
		if err := r.notify(group); err != nil {
			return err
		}
	}
	return nil
}

func (r *run) pruneOne(res manifest.Resource) error {
	switch res.Kind {
	case manifest.KindFile:
		if u := res.File.Unit; u != "" && !r.stopped[u] {
			// A unit without [Install] has no service resource to stop it.
			r.stopped[u] = true
			if err := r.stopUnit(u); err != nil {
				return err
			}
		}
		if err := r.backupFile(res.File.Path); err != nil {
			return err
		}
		return r.h.Remove(res.File.Path)
	case manifest.KindDownload:
		return r.h.Remove(res.Download.Path)
	case manifest.KindService:
		r.stopped[res.Service.Unit] = true
		_, err := r.h.Run(r.ctx, host.Cmd{Argv: []string{"systemctl", "disable", "--now", res.Service.Unit}})
		return ignoreMissingUnit(err)
	case manifest.KindDatabaseUser:
		_, err := r.h.Run(r.ctx, mariadbCmd(res.DatabaseUser.Server, dropUserSQL(res.DatabaseUser)))
		return err
	case manifest.KindCert:
		if res.Cert.Provider == manifest.CertLetsEncrypt {
			_, _ = r.h.Run(r.ctx, host.Cmd{Argv: []string{"certbot", "delete", "--non-interactive", "--cert-name", res.Name}})
		}
		return errors.Join(r.h.Remove(res.Cert.CertPath), r.h.Remove(res.Cert.KeyPath))
	}
	return nil
}

func (r *run) stopUnit(unit string) error {
	_, err := r.h.Run(r.ctx, host.Cmd{Argv: []string{"systemctl", "stop", unit}})
	return ignoreMissingUnit(err)
}

func ignoreMissingUnit(err error) error {
	if err != nil && (strings.Contains(err.Error(), "not loaded") || strings.Contains(err.Error(), "does not exist")) {
		return nil
	}
	return err
}

// record writes the new generation. On failure only completed resources
// are trusted; everything else attempted stays owned (pending).
func (r *run) record(desired *manifest.Manifest, prev *state.Generation, number int, failed bool) error {
	g := &state.Generation{Number: number, Failed: failed}
	// Generations are stored redacted: they record what was applied (by
	// digest), never the secrets themselves, so rotated passwords don't
	// linger in history.
	if !failed {
		g.Manifest = desired.Redacted()
	} else {
		applied := manifest.New(desired.Node, desired.Roles)
		var pending []manifest.Resource
		for i := range desired.Resources {
			res := desired.Resources[i]
			if r.completed[res.ID()] {
				_ = applied.Add(res.Redacted())
			} else if old := prev.Applied(res.ID()); old != nil {
				_ = applied.Add(*old)
				pending = append(pending, res.Redacted())
			} else {
				pending = append(pending, res.Redacted())
			}
		}
		// Keep previously owned resources that weren't pruned yet.
		for id, res := range prev.Owned() {
			if desired.Get(id) == nil && applied.Get(id) == nil {
				pending = append(pending, res)
			}
		}
		applied.Finalize()
		slices.SortFunc(pending, func(a, b manifest.Resource) int { return strings.Compare(a.ID().String(), b.ID().String()) })
		g.Manifest, g.Pending = applied, pending
	}
	b, err := g.Marshal()
	if err != nil {
		return err
	}
	if err := r.h.WriteFile(state.GenerationPath(number), b, 0o600, "root", "root", nil); err != nil {
		return err
	}
	return r.h.WriteFile(state.CurrentPath, b, 0o600, "root", "root", nil)
}
