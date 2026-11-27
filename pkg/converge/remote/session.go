package remote

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/roots/trellis-cli/pkg/converge/agent"
	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/engine"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/release"
)

// Server is an environment's server: its manifest, connection and plan.
type Server struct {
	Name     string
	Manifest *manifest.Manifest
	Target   Target
	Binary   Binary
	Plan     *engine.Plan
}

// Session runs plan/apply/deploy against an environment's server.
type Session struct {
	SSH *SSH
	// FindBinary returns the agent binary for a server architecture.
	FindBinary func(arch string) (Binary, error)
	Out        io.Writer
	// Host and Port override the environment's address (e.g. a local VM).
	Host string
	Port int
	// User logs in: root to provision, web to deploy, or another user
	// with passwordless sudo (a Lima VM's, --user).
	User string
	// RunAs runs the agent as this user instead of root: deploys and
	// rollbacks run as web.
	RunAs string
}

// Connect logs in to env's server, checks how to get root, and makes sure
// the agent is installed.
func (s *Session) Connect(ctx context.Context, env *config.Environment, m *manifest.Manifest) (*Server, error) {
	host, port := env.Host, env.Port
	if s.Host != "" {
		host = s.Host
	}
	if s.Port != 0 {
		port = s.Port
	}
	if host == "" {
		return nil, fmt.Errorf("environment %s has no host (use --lima or --host for a local VM)", env.Name)
	}
	srv := &Server{Name: env.ServerName(), Manifest: m}
	if srv.Name != host {
		srv.Name += " " + host
	}
	t, err := s.SSH.Connect(ctx, host, port, s.User)
	if err != nil {
		return nil, err
	}
	t.RunAs = s.RunAs
	if t.User != t.RunAs && t.User != "root" {
		if err := s.SSH.CheckSudo(ctx, t); err != nil {
			return nil, err
		}
	}
	srv.Target = t
	arch, err := s.SSH.Arch(ctx, t)
	if err != nil {
		return nil, err
	}
	if srv.Binary, err = s.FindBinary(arch); err != nil {
		return nil, err
	}
	if err := s.SSH.EnsureAgent(ctx, t, srv.Binary); err != nil {
		return nil, err
	}
	return srv, nil
}

// Plan asks the agent for a plan and renders it.
func (s *Session) Plan(ctx context.Context, srv *Server) error {
	srv.Plan = nil
	err := s.SSH.Call(ctx, srv.Target, srv.Binary, agent.Request{Mode: agent.ModePlan, Manifest: srv.Manifest}, func(m agent.Message) {
		if m.Type == "plan" {
			srv.Plan = m.Plan
		}
	})
	if err != nil {
		return fmt.Errorf("%s: %w", srv.Name, err)
	}
	if srv.Plan == nil {
		return fmt.Errorf("%s: agent returned no plan", srv.Name)
	}
	srv.Plan.Render(s.Out, srv.Name)
	_, _ = fmt.Fprintln(s.Out)
	return nil
}

// Apply applies the reviewed plan. The agent refuses if its plan differs
// from the reviewed one (the server changed in between).
func (s *Session) Apply(ctx context.Context, srv *Server) error {
	if srv.Plan == nil {
		return errors.New("apply needs a reviewed plan")
	}
	if srv.Plan.Empty() {
		return nil
	}
	fmt.Fprintf(s.Out, "%s\n", srv.Name)
	err := s.SSH.Call(ctx, srv.Target, srv.Binary, agent.Request{
		Mode: agent.ModeApply, Manifest: srv.Manifest, Expect: srv.Plan.Hash(),
	}, func(m agent.Message) {
		switch m.Type {
		case "event":
			_, _ = fmt.Fprintln(s.Out, m.Event.String())
		case "result":
			fmt.Fprintf(s.Out, "  ✓ generation %d applied\n\n", m.Result.Generation)
		}
	})
	if err != nil {
		return fmt.Errorf("%s: %w", srv.Name, err)
	}
	return nil
}

// Upgrade runs unattended-upgrades on the server now.
func (s *Session) Upgrade(ctx context.Context, srv *Server) (*engine.UpgradeResult, error) {
	fmt.Fprintf(s.Out, "%s\n", srv.Name)
	var res *engine.UpgradeResult
	err := s.SSH.Call(ctx, srv.Target, srv.Binary, agent.Request{Mode: agent.ModeUpgrade, Manifest: srv.Manifest}, func(m agent.Message) {
		switch m.Type {
		case "event":
			_, _ = fmt.Fprintln(s.Out, m.Event.String())
		case "result":
			res = m.Upgrade
		}
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", srv.Name, err)
	}
	if res == nil {
		return nil, fmt.Errorf("%s: agent returned no result", srv.Name)
	}
	return res, nil
}

// Artifact is a built release to upload.
type Artifact struct {
	Data   []byte
	SHA256 string
	Path   string // content-addressed path on the server
}

func (s *Session) releaseCall(ctx context.Context, srv *Server, req agent.Request, done func(*agent.ReleaseResult)) error {
	err := s.SSH.Call(ctx, srv.Target, srv.Binary, req, func(m agent.Message) {
		switch m.Type {
		case "event":
			_, _ = fmt.Fprintln(s.Out, m.Event.String())
		case "result":
			if m.Release != nil {
				done(m.Release)
			}
		}
	})
	if err != nil {
		return fmt.Errorf("%s: %w", srv.Name, err)
	}
	return nil
}

// Deploy uploads art (unless the server has it) and activates spec.
func (s *Session) Deploy(ctx context.Context, srv *Server, art Artifact, spec release.Spec) error {
	fmt.Fprintf(s.Out, "%s\n", srv.Name)
	fmt.Fprintf(s.Out, "  upload   %s (%.1f MB)\n", art.Path, float64(len(art.Data))/1e6)
	if err := s.SSH.Upload(ctx, srv.Target, art.Data, art.SHA256, art.Path); err != nil {
		return fmt.Errorf("%s: upload: %w", srv.Name, err)
	}
	return s.releaseCall(ctx, srv, agent.Request{Mode: agent.ModeDeploy, Deploy: &spec}, func(res *agent.ReleaseResult) {
		fmt.Fprintf(s.Out, "  ✓ %s is live\n\n", res.To)
	})
}

// Rollback switches site back to its previous release.
func (s *Session) Rollback(ctx context.Context, srv *Server, site, phpUnit string) error {
	fmt.Fprintf(s.Out, "%s\n", srv.Name)
	return s.releaseCall(ctx, srv, agent.Request{Mode: agent.ModeRollback, Rollback: &agent.RollbackRequest{Site: site, PHPUnit: phpUnit}},
		func(res *agent.ReleaseResult) {
			fmt.Fprintf(s.Out, "  ✓ rolled back %s → %s\n\n", res.From, res.To)
		})
}
