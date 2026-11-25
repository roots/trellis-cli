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

	"github.com/roots/trellis-cli/pkg/converge/engine"
	"github.com/roots/trellis-cli/pkg/converge/host"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
)

// Protocol version; bump on incompatible changes.
const Protocol = 1

type Mode string

const (
	ModePlan  Mode = "plan"
	ModeApply Mode = "apply"
)

type Request struct {
	Protocol int                `json:"protocol"`
	Mode     Mode               `json:"mode"`
	Manifest *manifest.Manifest `json:"manifest"`
	// Expect is the plan hash the user approved. Apply refuses to run a
	// different plan (the host changed between plan and apply).
	Expect string `json:"expect,omitempty"`
}

type Message struct {
	Type   string         `json:"type"` // plan | event | result | error
	Plan   *engine.Plan   `json:"plan,omitempty"`
	Event  *engine.Event  `json:"event,omitempty"`
	Result *engine.Result `json:"result,omitempty"`
	Error  string         `json:"error,omitempty"`
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
