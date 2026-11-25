package remote

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"

	"github.com/roots/trellis-cli/pkg/converge/agent"
	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/engine"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
)

// NodeRun is one node's manifest, connection and plan.
type NodeRun struct {
	Node     config.Node
	Manifest *manifest.Manifest
	Target   Target
	Binary   Binary
	Plan     *engine.Plan
}

func (n *NodeRun) Title() string {
	if n.Node.Name == n.Target.Host {
		return n.Node.Name
	}
	return n.Node.Name + " " + n.Target.Host
}

// Session orchestrates plan/apply across an environment's nodes.
type Session struct {
	SSH *SSH
	// FindBinary returns the agent binary for a node architecture.
	FindBinary func(arch string) (Binary, error)
	Out        io.Writer
	// Host overrides the address of a single-node environment (e.g. a
	// development VM's IP).
	Host string
	// Users to try when a node doesn't set one.
	Users []string
}

// Prepare pairs manifests with nodes, orders them (db-only nodes first, so
// databases exist before web nodes need them), connects to each and makes
// sure the agent is installed. Nodes are prepared concurrently.
func (s *Session) Prepare(ctx context.Context, env *config.Environment, ms []*manifest.Manifest) ([]*NodeRun, error) {
	if s.Host != "" && len(env.Nodes) != 1 {
		return nil, errors.New("--host can only be used with single-node environments")
	}
	byName := map[string]*manifest.Manifest{}
	for _, m := range ms {
		byName[m.Node] = m
	}
	var runs []*NodeRun
	for _, n := range env.Nodes {
		if s.Host != "" {
			n.Host = s.Host
		}
		if n.Host == "" {
			return nil, fmt.Errorf("node %s has no host (pass --host for the development VM)", n.Name)
		}
		runs = append(runs, &NodeRun{Node: n, Manifest: byName[n.Name]})
	}
	slices.SortStableFunc(runs, func(a, b *NodeRun) int {
		return dbFirst(a.Node) - dbFirst(b.Node)
	})

	err := parallel(runs, func(r *NodeRun) error {
		users := s.Users
		if r.Node.User != "" {
			users = []string{r.Node.User}
		}
		t, err := s.SSH.Connect(ctx, r.Node.Host, r.Node.Port, users)
		if err != nil {
			return err
		}
		r.Target = t
		arch, err := s.SSH.Arch(ctx, t)
		if err != nil {
			return err
		}
		if r.Binary, err = s.FindBinary(arch); err != nil {
			return err
		}
		return s.SSH.EnsureAgent(ctx, t, r.Binary)
	})
	return runs, err
}

func dbFirst(n config.Node) int {
	if n.HasRole(config.RoleDB) && !n.HasRole(config.RoleWeb) {
		return 0
	}
	return 1
}

// Plan computes every node's plan concurrently and renders them in order.
func (s *Session) Plan(ctx context.Context, runs []*NodeRun) error {
	err := parallel(runs, func(r *NodeRun) error {
		return s.SSH.Call(ctx, r.Target, r.Binary, agent.Request{Mode: agent.ModePlan, Manifest: r.Manifest}, func(m agent.Message) {
			if m.Type == "plan" {
				r.Plan = m.Plan
			}
		})
	})
	if err != nil {
		return err
	}
	for _, r := range runs {
		if r.Plan == nil {
			return fmt.Errorf("%s: agent returned no plan", r.Title())
		}
		r.Plan.Render(s.Out, r.Title())
		_, _ = fmt.Fprintln(s.Out)
	}
	return nil
}

// Empty reports whether no node has changes.
func Empty(runs []*NodeRun) bool {
	for _, r := range runs {
		if r.Plan != nil && !r.Plan.Empty() {
			return false
		}
	}
	return true
}

// Apply applies each node's reviewed plan in order, stopping at the first
// failure. Each apply is bound to the reviewed plan's hash.
func (s *Session) Apply(ctx context.Context, runs []*NodeRun) error {
	for _, r := range runs {
		if r.Plan.Empty() {
			continue
		}
		fmt.Fprintf(s.Out, "%s\n", r.Title())
		err := s.SSH.Call(ctx, r.Target, r.Binary, agent.Request{
			Mode: agent.ModeApply, Manifest: r.Manifest, Expect: r.Plan.Hash(),
		}, func(m agent.Message) {
			switch m.Type {
			case "event":
				_, _ = fmt.Fprintln(s.Out, m.Event.String())
			case "result":
				fmt.Fprintf(s.Out, "  ✓ generation %d applied\n\n", m.Result.Generation)
			}
		})
		if err != nil {
			return fmt.Errorf("%s: %w", r.Title(), err)
		}
	}
	return nil
}

// parallel runs f for each run concurrently and joins the errors.
func parallel(runs []*NodeRun, f func(*NodeRun) error) error {
	var wg sync.WaitGroup
	errs := make([]error, len(runs))
	for i, r := range runs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := f(r); err != nil {
				errs[i] = fmt.Errorf("%s: %w", r.Node.Name, err)
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}
