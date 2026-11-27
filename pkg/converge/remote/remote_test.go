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
	// sudoPassword, if set, is required by sudo for non-root users.
	sudoPassword string
	passwords    int      // password lines consumed by sudo -S
	agentUsers   []string // who each agent run ran as
}

var (
	agentPathRe    = regexp.MustCompile(`(/usr/local/lib/trellis|/home/web/\.local/lib/trellis)/agent-[0-9a-f]+`)
	artifactPathRe = regexp.MustCompile(`/home/web/\.cache/trellis/[0-9a-f]+\.tar\.gz`)
)

// readLine reads one line byte by byte, leaving the rest of r unread (as
// sudo -S does).
func readLine(r io.Reader) string {
	var b []byte
	one := make([]byte, 1)
	for {
		n, err := r.Read(one)
		if n == 1 && one[0] != '\n' {
			b = append(b, one[0])
		}
		if err != nil || (n == 1 && one[0] == '\n') {
			return string(b)
		}
	}
}

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
	// Run as web: from root, or through sudo -u (a password or not).
	if c, ok := strings.CutPrefix(cmd, "runuser -u web -- "); ok && root {
		cmd, root, web = c, false, true
	}
	for _, p := range []string{"sudo -n -u web ", "sudo -k -S -p '' -u web "} {
		c, ok := strings.CutPrefix(cmd, p)
		if !ok {
			continue
		}
		if strings.Contains(p, "-S") && readLine(stdin) != f.sudoPassword {
			return errors.New("sudo: incorrect password")
		}
		if !strings.Contains(p, "-S") && f.sudoPassword != "" {
			return errors.New("sudo: a password is required")
		}
		cmd, web = c, true
	}
	switch {
	case web:
	case strings.HasPrefix(cmd, "sudo -n "):
		if !root && f.sudoPassword != "" {
			_, _ = io.WriteString(stderr, "sudo: a password is required")
			return errors.New("exit status 1")
		}
		cmd, root = strings.TrimPrefix(cmd, "sudo -n "), true
	case strings.HasPrefix(cmd, "sudo -k -S -p '' "):
		if readLine(stdin) != f.sudoPassword {
			return errors.New("sudo: incorrect password")
		}
		f.mu.Lock()
		f.passwords++
		f.mu.Unlock()
		cmd, root = strings.TrimPrefix(cmd, "sudo -k -S -p '' "), true
	}
	switch {
	case cmd == "true":
		return nil
	case strings.HasPrefix(cmd, "uname -m; "): // the connect probe
		access := "root"
		if !root {
			access = "nopasswd"
			if f.sudoPassword != "" || web {
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

	// Fresh server: only root can log in.
	target, err := s.Connect(ctx, "203.0.113.10", 22, []string{"admin", "root"})
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
	if len(f.calls) != 3 || !strings.Contains(f.calls[0], "admin@") || !strings.Contains(f.calls[1], "root@203.0.113.10 uname -m; ") {
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

	// After hardening, admin works and the agent runs via sudo.
	f.users = map[string]bool{"admin": true}
	target, err = s.Connect(ctx, "203.0.113.10", 22, []string{"admin", "root"})
	if err != nil || target.User != "admin" {
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

func TestSudoPasswordIsVerifiedAndSentOnlyWhenNeeded(t *testing.T) {
	ctx := context.Background()
	m := manifest.New("n", []string{"web"})
	if err := m.Add(manifest.NewPackage("nginx", "")); err != nil {
		t.Fatal(err)
	}
	m.Finalize()
	b := binary(t)

	// Passwordless sudo (development): no password is ever sent.
	f := &fakeSSH{host: hosttest.New(), users: map[string]bool{"admin": true}, installed: map[string]bool{}}
	s := &remote.SSH{Exec: f}
	tgt := remote.Target{Host: "h", User: "admin"}
	if err := s.Elevate(ctx, &tgt, "pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAgent(ctx, tgt, b); err != nil {
		t.Fatal(err)
	}
	if f.passwords != 0 {
		t.Errorf("sent %d passwords to passwordless sudo", f.passwords)
	}

	// Password sudo: verified once, then sent with the upload and the call.
	f = &fakeSSH{host: hosttest.New(), users: map[string]bool{"admin": true}, installed: map[string]bool{}, sudoPassword: "pw"}
	s = &remote.SSH{Exec: f}
	tgt = remote.Target{Host: "h", User: "admin"}
	if err := s.Elevate(ctx, &tgt, ""); err == nil || !strings.Contains(err.Error(), "admin_password") {
		t.Errorf("missing password: %v", err)
	}
	if err := s.Elevate(ctx, &tgt, "wrong"); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Errorf("wrong password: %v", err)
	}
	if err := s.Elevate(ctx, &tgt, "pw"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureAgent(ctx, tgt, b); err != nil {
		t.Fatal(err)
	}
	var got []agent.Message
	if err := s.Call(ctx, tgt, b, agent.Request{Mode: agent.ModePlan, Manifest: m}, func(msg agent.Message) { got = append(got, msg) }); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Plan == nil {
		t.Fatalf("agent didn't get the request intact: %+v", got)
	}
	if f.passwords != 3 { // verify, upload, call
		t.Errorf("password lines = %d, want 3", f.passwords)
	}
}
