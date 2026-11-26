// Package remote runs the agent on a node over SSH.
//
// All SSH invocations share one multiplexed connection (ControlMaster), so
// a run costs one TCP+auth handshake. The agent (a separate, small binary
// embedded in trellis-cli) is uploaded compressed to a content-addressed
// path once and reused until it changes.
package remote

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/agent"
	"github.com/roots/trellis-cli/pkg/converge/agentbin"
)

// Exec runs a local process. It is the seam tests replace.
type Exec interface {
	Run(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error
}

// OSExec runs processes with os/exec.
type OSExec struct{}

func (OSExec) Run(ctx context.Context, argv []string, stdin io.Reader, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return cmd.Run()
}

// Target is a resolved SSH destination.
type Target struct {
	Host string
	Port int
	User string
	// sudoPassword is set by Elevate when the user's sudo needs a password.
	sudoPassword string
}

func (t Target) String() string { return t.User + "@" + t.Host }

// SSH builds and runs ssh commands.
type SSH struct {
	Exec    Exec
	Options []string // extra -o options
}

func NewSSH() *SSH { return &SSH{Exec: OSExec{}} }

// Argv returns the ssh command for running remote on t.
func (s *SSH) Argv(t Target, remote string) []string {
	argv := []string{"ssh",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=10",
		"-o", "ControlMaster=auto",
		"-o", "ControlPath=~/.ssh/trellis-%C",
		"-o", "ControlPersist=60",
	}
	for _, o := range s.Options {
		argv = append(argv, "-o", o)
	}
	if t.Port != 0 && t.Port != 22 {
		argv = append(argv, "-p", strconv.Itoa(t.Port))
	}
	return append(argv, t.User+"@"+t.Host, "--", remote)
}

// Run executes remote on t.
func (s *SSH) Run(ctx context.Context, t Target, remote string, stdin io.Reader, stdout io.Writer) error {
	var stderr bytes.Buffer
	if err := s.Exec.Run(ctx, s.Argv(t, remote), stdin, stdout, &stderr); err != nil {
		return fmt.Errorf("ssh %s: %w: %s", t, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func (s *SSH) output(ctx context.Context, t Target, remote string) (string, error) {
	var out bytes.Buffer
	err := s.Run(ctx, t, remote, nil, &out)
	return strings.TrimSpace(out.String()), err
}

// Connect returns the first of users that can log in. Trellis disables
// root login, so after the first run the admin user is the way in.
func (s *SSH) Connect(ctx context.Context, host string, port int, users []string) (Target, error) {
	var errs []error
	for _, u := range users {
		t := Target{Host: host, Port: port, User: u}
		if _, err := s.output(ctx, t, "true"); err == nil {
			return t, nil
		} else {
			errs = append(errs, err)
		}
	}
	return Target{}, fmt.Errorf("can't connect to %s as %s:\n%w", host, strings.Join(users, " or "), errors.Join(errs...))
}

// Arch returns the node's GOARCH.
func (s *SSH) Arch(ctx context.Context, t Target) (string, error) {
	out, err := s.output(ctx, t, "uname -m")
	if err != nil {
		return "", err
	}
	switch out {
	case "x86_64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	}
	return "", fmt.Errorf("%s: unsupported architecture %q", t, out)
}

// Binary is an agent executable for one architecture.
type Binary struct {
	SHA256 string // of the uncompressed executable; its content address
	Source string // where it came from, for messages
	// Gzipped reports whether Data is gzip-compressed; the node then
	// decompresses it while receiving it.
	Gzipped bool
	Data    []byte
}

// RemotePath is the content-addressed location on the node.
func (b Binary) RemotePath() string { return "/usr/local/lib/trellis/agent-" + b.SHA256[:16] }

// FindBinary returns the linux agent for arch: an explicit path, then
// $TRELLIS_AGENT_DIR/trellis-agent-linux-<arch>, then the agent embedded in
// this trellis-cli.
func FindBinary(explicit, arch string) (Binary, error) {
	path := explicit
	if path == "" {
		if dir := os.Getenv("TRELLIS_AGENT_DIR"); dir != "" {
			path = filepath.Join(dir, agentbin.Name(arch))
		}
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return Binary{}, err
		}
		sum := sha256.Sum256(b)
		return Binary{SHA256: hex.EncodeToString(sum[:]), Source: path, Data: b}, nil
	}
	if a, ok := agentbin.Get(arch); ok {
		return Binary{SHA256: a.SHA256, Source: "embedded", Gzipped: true, Data: a.Gzip}, nil
	}
	return Binary{}, fmt.Errorf("this trellis-cli has no embedded linux/%s agent; build them with\n  go generate ./pkg/converge/agentbin\nand rebuild trellis-cli, or pass --agent-binary / set TRELLIS_AGENT_DIR", arch)
}

// Elevate checks how t's user gets root. Root needs nothing; passwordless
// sudo is used as is; otherwise password is verified once and then sent as
// the first line of stdin on every elevated command (`sudo -S` consumes
// exactly that line; `-k` makes it always ask, so the line is never left
// for the command to read).
func (s *SSH) Elevate(ctx context.Context, t *Target, password string) error {
	if t.User == "root" {
		return nil
	}
	if _, err := s.output(ctx, *t, "sudo -n true"); err == nil {
		return nil
	}
	if password == "" {
		return fmt.Errorf("%s: sudo needs a password and none is configured (admin_password in the secrets file)", t)
	}
	if err := s.Run(ctx, *t, "sudo -k -S -p '' true", strings.NewReader(password+"\n"), io.Discard); err != nil {
		return fmt.Errorf("%s: sudo rejected admin_password from the secrets file", t)
	}
	t.sudoPassword = password
	return nil
}

func sudo(t Target) string {
	switch {
	case t.User == "root":
		return ""
	case t.sudoPassword != "":
		return "sudo -k -S -p '' "
	}
	return "sudo -n "
}

// elevated prefixes stdin with the sudo password when one is needed.
func elevated(t Target, stdin io.Reader) io.Reader {
	if t.sudoPassword == "" || t.User == "root" {
		return stdin
	}
	return io.MultiReader(strings.NewReader(t.sudoPassword+"\n"), stdin)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }

// EnsureAgent uploads b unless the node already has it.
func (s *SSH) EnsureAgent(ctx context.Context, t Target, b Binary) error {
	p := b.RemotePath()
	if _, err := s.output(ctx, t, "test -x "+p); err == nil {
		return nil
	}
	recv := "cat"
	if b.Gzipped {
		recv = "gzip -dc"
	}
	// The whole script runs as root, so a sudo password can precede the
	// binary on stdin.
	script := fmt.Sprintf(`set -e; t=$(mktemp); trap 'rm -f "$t"' EXIT; %s > "$t"; echo "%s  $t" | sha256sum -c --quiet; install -D -m 0755 "$t" %s`,
		recv, b.SHA256, p)
	return s.Run(ctx, t, sudo(t)+"sh -c "+shellQuote(script), elevated(t, bytes.NewReader(b.Data)), io.Discard)
}

// Call runs the agent with req and passes each streamed message to onMsg.
// A message of type "error" is returned as an error.
func (s *SSH) Call(ctx context.Context, t Target, b Binary, req agent.Request, onMsg func(agent.Message)) error {
	req.Protocol = agent.Protocol
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	var agentErr error
	go func() {
		sc := bufio.NewScanner(pr)
		sc.Buffer(nil, 64<<20)
		for sc.Scan() {
			var m agent.Message
			if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
				continue // tolerate stray output (e.g. motd)
			}
			if m.Type == "error" {
				agentErr = errors.New(m.Error)
			}
			onMsg(m)
		}
		_, _ = io.Copy(io.Discard, pr)
		done <- sc.Err()
	}()
	runErr := s.Run(ctx, t, sudo(t)+b.RemotePath(), elevated(t, bytes.NewReader(body)), pw)
	_ = pw.Close()
	scanErr := <-done
	if agentErr != nil {
		return agentErr
	}
	return errors.Join(runErr, scanErr)
}

// Upload copies data to path on t unless a file is already there. Paths are
// content-addressed (they contain data's sha256), so an existing file is
// the same file. The upload is verified and moved into place atomically.
func (s *SSH) Upload(ctx context.Context, t Target, data []byte, sha256hex, path string) error {
	if _, err := s.output(ctx, t, "test -f "+shellQuote(path)); err == nil {
		return nil
	}
	script := fmt.Sprintf(`set -e; d=$(dirname %[1]s); mkdir -p "$d"; f=$(mktemp "$d/.upload-XXXXXX"); trap 'rm -f "$f"' EXIT; cat > "$f"; echo "%[2]s  $f" | sha256sum -c --quiet; chmod 0644 "$f"; mv "$f" %[1]s`,
		shellQuote(path), sha256hex)
	return s.Run(ctx, t, sudo(t)+"sh -c "+shellQuote(script), elevated(t, bytes.NewReader(data)), io.Discard)
}
