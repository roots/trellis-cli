// Package agent is the server-side entry point. trellis-cli copies itself
// (a static linux binary) to the node and runs `trellis converge agent`
// over SSH with a Request on stdin; the agent streams Messages (JSON lines)
// on stdout. Keeping observe+plan+apply on the server means one SSH round
// trip per run instead of one per task.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/engine"
	"github.com/roots/trellis-cli/pkg/converge/host"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/release"
)

// Protocol version; bump on incompatible changes.
const Protocol = 3 // 2: deploy and rollback; 3: upgrade

type Mode string

const (
	ModePlan     Mode = "plan"
	ModeApply    Mode = "apply"
	ModeDeploy   Mode = "deploy"
	ModeRollback Mode = "rollback"
	ModeUpgrade  Mode = "upgrade"
)

type Request struct {
	Protocol int                `json:"protocol"`
	Mode     Mode               `json:"mode"`
	Manifest *manifest.Manifest `json:"manifest"`
	// Expect is the plan hash the user approved. Apply refuses to run a
	// different plan (the host changed between plan and apply).
	Expect string `json:"expect,omitempty"`

	Deploy   *release.Spec    `json:"deploy,omitempty"`
	Rollback *RollbackRequest `json:"rollback,omitempty"`
}

// RollbackRequest switches a site back to its previous release.
type RollbackRequest struct {
	Site    string `json:"site"`
	PHPUnit string `json:"php_unit"`
}

// ReleaseResult reports a deploy or rollback.
type ReleaseResult struct {
	From string `json:"from,omitempty"`
	To   string `json:"to"`
}

type Message struct {
	Type    string                `json:"type"` // plan | event | result | error
	Plan    *engine.Plan          `json:"plan,omitempty"`
	Event   *engine.Event         `json:"event,omitempty"`
	Result  *engine.Result        `json:"result,omitempty"`
	Release *ReleaseResult        `json:"release,omitempty"`
	Upgrade *engine.UpgradeResult `json:"upgrade,omitempty"`
	Error   string                `json:"error,omitempty"`
}

// Locker serialises concurrent runs on a host.
type Locker func() (unlock func(), err error)

// Serve handles one request.
func Serve(ctx context.Context, h host.Host, lock Locker, in io.Reader, out io.Writer) error {
	enc := json.NewEncoder(out)
	send := func(m Message) { _ = enc.Encode(m) }
	err := serve(ctx, h, lock, in, send)
	if err != nil {
		send(Message{Type: "error", Error: err.Error()})
	}
	return err
}

func serve(ctx context.Context, h host.Host, lock Locker, in io.Reader, send func(Message)) error {
	var req Request
	if err := json.NewDecoder(in).Decode(&req); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	if req.Protocol != Protocol {
		return fmt.Errorf("protocol mismatch: client %d, agent %d (re-upload the agent)", req.Protocol, Protocol)
	}
	if req.Mode == ModeDeploy || req.Mode == ModeRollback {
		return serveRelease(ctx, h, lock, req, send)
	}
	if req.Manifest == nil {
		return fmt.Errorf("request has no manifest")
	}
	if err := req.Manifest.Validate(); err != nil {
		return err
	}
	req.Manifest.Finalize()

	if lock != nil {
		unlock, err := lock()
		if err != nil {
			return err
		}
		defer unlock()
	}
	if err := checkOS(h, req.Manifest.OS); err != nil {
		return err
	}
	if req.Mode == ModeUpgrade {
		res, err := engine.Upgrade(ctx, h, func(e engine.Event) { send(Message{Type: "event", Event: &e}) })
		if err != nil {
			return err
		}
		send(Message{Type: "result", Upgrade: res})
		return nil
	}

	obs, err := engine.Observe(ctx, h, req.Manifest)
	if err != nil {
		return err
	}
	p := engine.Compute(req.Manifest, obs)
	send(Message{Type: "plan", Plan: p.Redacted()})
	if req.Mode != ModeApply {
		return nil
	}
	if req.Expect != "" && req.Expect != p.Hash() {
		return fmt.Errorf("the host changed since the plan was reviewed (expected plan %s, now %s); run plan again", req.Expect, p.Hash())
	}
	prev := obs.Previous
	if p.Empty() && prev != nil && !prev.Failed && prev.Manifest.Hash() == p.ManifestHash {
		send(Message{Type: "result", Result: &engine.Result{Generation: prev.Number}})
		return nil
	}
	res, err := engine.Apply(ctx, h, req.Manifest, p, prev, engine.Options{
		Log: func(e engine.Event) { send(Message{Type: "event", Event: &e}) },
	})
	if err != nil {
		return err
	}
	send(Message{Type: "result", Result: res})
	return nil
}

// serveRelease handles deploy and rollback. They take the same lock as
// apply, so a deploy never runs during provisioning.
func serveRelease(ctx context.Context, h host.Host, lock Locker, req Request, send func(Message)) error {
	if lock != nil {
		unlock, err := lock()
		if err != nil {
			return err
		}
		defer unlock()
	}
	log := func(step, detail string) {
		send(Message{Type: "event", Event: &engine.Event{Op: string(req.Mode), Target: step, Message: detail}})
	}
	switch req.Mode {
	case ModeDeploy:
		if req.Deploy == nil {
			return fmt.Errorf("deploy request has no spec")
		}
		if err := release.Activate(ctx, h, *req.Deploy, log); err != nil {
			return err
		}
		send(Message{Type: "result", Release: &ReleaseResult{To: req.Deploy.ID}})
	case ModeRollback:
		if req.Rollback == nil {
			return fmt.Errorf("rollback request has no site")
		}
		from, to, err := release.Rollback(ctx, h, req.Rollback.Site, req.Rollback.PHPUnit, log)
		if err != nil {
			return err
		}
		send(Message{Type: "result", Release: &ReleaseResult{From: from, To: to}})
	}
	return nil
}

// checkOS refuses to plan for a different operating system than the
// manifest was compiled for: repositories and packages are OS-specific, and
// an in-place OS upgrade should be a deliberate config change.
func checkOS(h host.Host, want string) error {
	if want == "" {
		return nil
	}
	b, err := h.ReadFile("/etc/os-release")
	if err != nil {
		return fmt.Errorf("read /etc/os-release: %w", err)
	}
	have := OSRelease(b)
	if have != want {
		return fmt.Errorf("the server runs %s but trellis.yml says os: %s", have, want)
	}
	return nil
}

// OSRelease returns "<ID>-<VERSION_ID>" (e.g. ubuntu-24.04, debian-13)
// from /etc/os-release.
func OSRelease(b []byte) string {
	var id, version string
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			id = v
		case "VERSION_ID":
			version = v
		}
	}
	return id + "-" + version
}
