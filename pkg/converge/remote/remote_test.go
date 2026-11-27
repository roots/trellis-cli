package remote_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/roots/trellis-cli/pkg/converge/agent"
	"github.com/roots/trellis-cli/pkg/converge/host/hosttest"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/remote"
)

// fakeSSH simulates `ssh user@host -- <remote>` by dispatching on the
// remote command, running the agent in-process against a fake host. This
// exercises the real client, protocol and agent without a network or VM.
type fakeSSH struct {
	mu        sync.Mutex
	host      *hosttest.Fake
	hosts     map[string]*hosttest.Fake // by address; falls back to host
	users     map[string]bool           // users allowed to log in
	installed map[string]bool
	uploads   int
	calls     []string
	// passwordSudo makes sudo ask non-root users for a password (which
	// Trellis never sends), so sudo -n fails.
	passwordSudo bool
	agentUsers   []string // who each agent run ran as
}

var (
	agentPathRe    = regexp.MustCompile(`(/usr/local/lib/trellis|/home/web/\.local/lib/trellis)/agent-[0-9a-f]+`)
	artifactPathRe = regexp.MustCompile(`/home/web/\.cache/trellis/[0-9a-f]+\.tar\.gz`)
)

func (f *fakeSSH) Run(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	dest, cmd := argv[len(argv)-3], argv[len(argv)-1]
	user, addr, _ := strings.Cut(dest, "@")
	f.mu.Lock()
	f.calls = append(f.calls, dest+" "+cmd)
	h := f.host
	if hh, ok := f.hosts[addr]; ok {
		h = hh
	}
	f.mu.Unlock()
	if !f.users[user] {
		_, _ = io.WriteString(stderr, "Permission denied (publickey).")
		return errors.New("exit status 255")
	}
	root, web := user == "root", user == "web"
	// Run as web: from root, or through sudo -u.
	if c, ok := strings.CutPrefix(cmd, "runuser -u web -- "); ok && root {
		cmd, root, web = c, false, true
	}
	for _, p := range []string{"sudo -n -u web ", "sudo -n "} {
		c, ok := strings.CutPrefix(cmd, p)
		if !ok || web {
			continue
		}
		if !root && f.passwordSudo {
			_, _ = io.WriteString(stderr, "sudo: a password is required")
			return errors.New("exit status 1")
		}
		if p == "sudo -n " {
			cmd, root = c, true
		} else {
			cmd, root, web = c, false, true
		}
		break
	}
	switch {
	case cmd == "true":
		return nil
	case strings.HasPrefix(cmd, "uname -m; "): // the connect probe
		access := "root"
		if !root {
			access = "nopasswd"
			if f.passwordSudo || web {
				access = "password"
			}
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		_, _ = io.WriteString(stdout, "x86_64\n"+access+"\n")
		for p := range f.installed {
			if a, ok := strings.CutPrefix(p, addr); ok && agentPathRe.MatchString(a) {
				_, _ = io.WriteString(stdout, a+"\n")
			}
		}
		return nil
	case strings.HasPrefix(cmd, "test -f "):
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.installed[addr+strings.Trim(strings.TrimPrefix(cmd, "test -f "), "'")] {
			return nil
		}
		return errors.New("exit status 1")
	case strings.Contains(cmd, ".upload-XXXXXX"):
		if !web {
			return errors.New("artifacts are uploaded as web")
		}
		_, _ = io.Copy(io.Discard, stdin)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.uploads++
		f.installed[addr+artifactPathRe.FindString(cmd)] = true
		return nil
	case strings.HasPrefix(cmd, "test -x "):
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.installed[addr+strings.TrimPrefix(cmd, "test -x ")] {
			return nil
		}
		return errors.New("exit status 1")
	case strings.Contains(cmd, "install -D -m 0755"):
		if !root && (!web || !strings.Contains(cmd, "/home/web/")) {
			return errors.New("install needs root")
		}
		_, _ = io.Copy(io.Discard, stdin)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.uploads++
		f.installed[addr+agentPathRe.FindString(cmd)] = true
		return nil
	case agentPathRe.MatchString(cmd):
		if !root && !web {
			return errors.New("agent must run as root or web")
		}
		f.mu.Lock()
		f.agentUsers = append(f.agentUsers, map[bool]string{true: "web", false: "root"}[web])
		f.mu.Unlock()
		return agent.Serve(ctx, h, nil, stdin, stdout)
	}
	return errors.New("unexpected command " + cmd)
}

func binary(t *testing.T) remote.Binary {
	t.Helper()
	p := filepath.Join(t.TempDir(), "trellis-agent-linux-amd64")
	if err := os.WriteFile(p, []byte("agent binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := remote.FindBinary(p, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestArgvMultiplexes(t *testing.T) {
	s := &remote.SSH{Options: []string{"StrictHostKeyChecking=accept-new"}}
	got := strings.Join(s.Argv(remote.Target{Host: "h", Port: 2222, User: "admin"}, "true"), " ")
	for _, want := range []string{"ControlMaster=auto", "ControlPersist=60", "BatchMode=yes", "-p 2222", "StrictHostKeyChecking=accept-new", "admin@h -- true"} {
		if !strings.Contains(got, want) {
			t.Errorf("argv missing %q: %s", want, got)
		}
	}
}

func TestEndToEndOverFakeSSH(t *testing.T) {
	f := &fakeSSH{host: hosttest.New(), users: map[string]bool{"root": true}, installed: map[string]bool{}}
	s := &remote.SSH{Exec: f}
	ctx := context.Background()
	b := binary(t)

	target, err := s.Connect(ctx, "203.0.113.10", 22, "root")
	if err != nil || target.User != "root" {
		t.Fatalf("connect: %v %v", target, err)
	}
	if arch, err := s.Arch(ctx, target); err != nil || arch != "amd64" {
		t.Fatalf("arch: %v %v", arch, err)
	}
	for range 2 {
		if err := s.EnsureAgent(ctx, target, b); err != nil {
			t.Fatal(err)
		}
	}
	if f.uploads != 1 {
		t.Errorf("agent uploaded %d times, want 1 (content-addressed)", f.uploads)
	}
	// One round trip before the agent runs: the probe, then the upload.
	if len(f.calls) != 2 || !strings.Contains(f.calls[0], "root@203.0.113.10 uname -m; ") {
		t.Errorf("setup calls = %q", f.calls)
	}

	m := manifest.New("n", nil)
	_ = m.Add(manifest.NewRootFile("/etc/motd", "hi\n", 0o644))
	m.Finalize()

	var plan *agent.Message
	if err := s.Call(ctx, target, b, agent.Request{Mode: agent.ModePlan, Manifest: m}, func(msg agent.Message) {
		if msg.Type == "plan" {
			plan = &msg
		}
	}); err != nil {
		t.Fatal(err)
	}
	if plan == nil || len(plan.Plan.Actions) != 1 {
		t.Fatalf("plan = %+v", plan)
	}

	var result *agent.Message
	err = s.Call(ctx, target, b, agent.Request{Mode: agent.ModeApply, Manifest: m, Expect: plan.Plan.Hash()}, func(msg agent.Message) {
		if msg.Type == "result" {
			result = &msg
		}
	})
	if err != nil || result == nil {
		t.Fatalf("apply: %v %+v", err, result)
	}
	if string(f.host.Files["/etc/motd"].Data) != "hi\n" {
		t.Error("file not applied through the agent")
	}

	// --user admin: the agent runs through passwordless sudo.
	f.users = map[string]bool{"admin": true}
	target, err = s.Connect(ctx, "203.0.113.10", 22, "admin")
	if err != nil || s.CheckSudo(ctx, target) != nil {
		t.Fatalf("connect as admin: %v %v", target, err)
	}
	if err := s.Call(ctx, target, b, agent.Request{Mode: agent.ModePlan, Manifest: m}, func(agent.Message) {}); err != nil {
		t.Fatalf("plan as admin: %v", err)
	}
}

func TestAgentErrorsPropagate(t *testing.T) {
	f := &fakeSSH{host: hosttest.New(), users: map[string]bool{"root": true}, installed: map[string]bool{}}
	s := &remote.SSH{Exec: f}
	m := manifest.New("n", nil)
	_ = m.Add(manifest.NewRootFile("/etc/motd", "hi\n", 0o644))
	m.Finalize()
	err := s.Call(context.Background(), remote.Target{Host: "h", User: "root"}, binary(t),
		agent.Request{Mode: agent.ModeApply, Manifest: m, Expect: "stale"}, func(agent.Message) {})
	if err == nil || !strings.Contains(err.Error(), "changed since the plan was reviewed") {
		t.Fatalf("expected agent error, got %v", err)
	}
}

func TestEmbeddedAgentUploadsCompressed(t *testing.T) {
	b, err := remote.FindBinary("", "amd64")
	if err != nil {
		t.Skip("no embedded agents (run go generate ./pkg/converge/agentbin)")
	}
	if !b.Gzipped || len(b.SHA256) != 64 {
		t.Fatalf("embedded binary = %s gzipped=%v sha=%q", b.Source, b.Gzipped, b.SHA256)
	}
	f := &fakeSSH{host: hosttest.New(), users: map[string]bool{"admin": true}, installed: map[string]bool{}}
	s := &remote.SSH{Exec: f}
	tgt := remote.Target{Host: "h", User: "admin"}
	if err := s.EnsureAgent(context.Background(), tgt, b); err != nil {
		t.Fatal(err)
	}
	if got := f.calls[len(f.calls)-1]; !strings.Contains(got, "gzip -dc >") || !strings.Contains(got, "admin@h sudo -n sh -c ") {
		t.Errorf("upload command = %s", got)
	}
}

func TestExplicitAgentOverridesEmbedded(t *testing.T) {
	b := binary(t)
	if b.Gzipped || !strings.HasSuffix(b.Source, "trellis-agent-linux-amd64") {
		t.Fatalf("binary = %+v", b)
	}
}

// Trellis never sends a sudo password: a user whose sudo needs one is
// refused up front, and a failed login says how to get in.
func TestConnectErrors(t *testing.T) {
	ctx := context.Background()
	f := &fakeSSH{host: hosttest.New(), users: map[string]bool{"ubuntu": true}, installed: map[string]bool{}, passwordSudo: true}
	s := &remote.SSH{Exec: f}
	tgt, err := s.Connect(ctx, "h", 22, "ubuntu")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.CheckSudo(ctx, tgt); err == nil || !strings.Contains(err.Error(), "sudo needs a password") {
		t.Errorf("password sudo: %v", err)
	}
	if _, err := s.Connect(ctx, "h", 22, "root"); err == nil || !strings.Contains(err.Error(), "access.admins") || !strings.Contains(err.Error(), "--user") {
		t.Errorf("root login refused: %v", err)
	}
}
