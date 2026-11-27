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
	// RunAs runs commands as this user instead of root (deploys run as
	// web): directly when logged in as it, otherwise through sudo -u.
	RunAs string
	// node is what Connect's probe learned, so the steps after it don't
	// each need a round trip.
	node *node
}

// node is what one round trip learns about a server.
type node struct {
	arch   string          // uname -m
	access string          // how the user gets root: root, nopasswd or password
	agents map[string]bool // installed agent paths
}

// Agent directories: the one apply installs into (root-owned), and web's
// own for deploys from a trellis-cli whose agent isn't installed yet.
const (
	systemAgentDir = "/usr/local/lib/trellis"
	webAgentDir    = "/home/web/.local/lib/trellis"
)

// probeScript reports the architecture, how the login user gets root, and
// the installed agents, one per line. It always succeeds once logged in.
const probeScript = `uname -m; ` +
	`if [ "$(id -u)" = 0 ]; then echo root; elif sudo -n true 2>/dev/null; then echo nopasswd; else echo password; fi; ` +
	`for f in ` + systemAgentDir + `/agent-* ` + webAgentDir + `/agent-*; do [ -x "$f" ] && echo "$f"; done; true`

func (s *SSH) probe(ctx context.Context, t Target) (*node, error) {
	if t.node != nil {
		return t.node, nil
	}
	out, err := s.output(ctx, t, probeScript)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("%s: unexpected probe output %q", t, out)
	}
	n := &node{arch: lines[0], access: lines[1], agents: map[string]bool{}}
	for _, p := range lines[2:] {
		n.agents[p] = true
	}
	return n, nil
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

// Connect logs in to host as user and probes the node.
func (s *SSH) Connect(ctx context.Context, host string, port int, user string) (Target, error) {
	t := Target{Host: host, Port: port, User: user}
	n, err := s.probe(ctx, t)
	if err != nil {
		hint := "use --user for an image whose default user isn't root (e.g. ubuntu)"
		if user == "root" {
			hint = "root logs in with an access.admins key (on a fresh server, the key your provider installed); " + hint
		}
		return Target{}, fmt.Errorf("can't log in to %s as %s (%s):\n%w", host, user, hint, err)
	}
	t.node = n
	return t, nil
}

// Arch returns the node's GOARCH.
func (s *SSH) Arch(ctx context.Context, t Target) (string, error) {
	n, err := s.probe(ctx, t)
	if err != nil {
		return "", err
	}
	out := n.arch
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
func (b Binary) RemotePath() string { return systemAgentDir + "/agent-" + b.SHA256[:16] }

// agentPath is where t runs b from: the installed agent when there is one
// (web can run it too), else, for a deploy, web's own copy.
func agentPath(t Target, n *node, b Binary) string {
	p := b.RemotePath()
	if t.RunAs == "" || n.agents[p] {
		return p
	}
	return webAgentDir + "/agent-" + b.SHA256[:16]
}

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

// CheckSudo makes sure t's user can get root: it is root, or has
// passwordless sudo.
func (s *SSH) CheckSudo(ctx context.Context, t Target) error {
	n, err := s.probe(ctx, t)
	if err != nil {
		return err
	}
	if n.access == "password" {
		return fmt.Errorf("%s: sudo needs a password; connect as root, or as a user with passwordless sudo", t)
	}
	return nil
}

// sudo is the prefix that runs a command as root, or as t.RunAs.
func sudo(t Target) string {
	switch {
	case t.RunAs != "" && t.RunAs == t.User:
		return ""
	case t.RunAs != "" && t.User == "root":
		return "runuser -u " + t.RunAs + " -- "
	case t.User == "root":
		return ""
	}
	p := "sudo -n "
	if t.RunAs != "" {
		p += "-u " + t.RunAs + " "
	}
	return p
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'" }

// EnsureAgent uploads b unless the node already has it.
func (s *SSH) EnsureAgent(ctx context.Context, t Target, b Binary) error {
	n, err := s.probe(ctx, t)
	if err != nil {
		return err
	}
	p := agentPath(t, n, b)
	if n.agents[p] {
		return nil
	}
	recv := "cat"
	if b.Gzipped {
		recv = "gzip -dc"
	}
	script := fmt.Sprintf(`set -e; t=$(mktemp); trap 'rm -f "$t"' EXIT; %s > "$t"; echo "%s  $t" | sha256sum -c --quiet; install -D -m 0755 "$t" %s`,
		recv, b.SHA256, p)
	if err := s.Run(ctx, t, sudo(t)+"sh -c "+shellQuote(script), bytes.NewReader(b.Data), io.Discard); err != nil {
		return err
	}
	n.agents[p] = true
	return nil
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
	n, err := s.probe(ctx, t)
	if err != nil {
		return err
	}
	runErr := s.Run(ctx, t, sudo(t)+agentPath(t, n, b), bytes.NewReader(body), pw)
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
	if err := s.Run(ctx, t, sudo(t)+"test -f "+shellQuote(path), nil, io.Discard); err == nil {
		return nil
	}
	script := fmt.Sprintf(`set -e; d=$(dirname %[1]s); mkdir -p "$d"; f=$(mktemp "$d/.upload-XXXXXX"); trap 'rm -f "$f"' EXIT; cat > "$f"; echo "%[2]s  $f" | sha256sum -c --quiet; chmod 0644 "$f"; mv "$f" %[1]s`,
		shellQuote(path), sha256hex)
	return s.Run(ctx, t, sudo(t)+"sh -c "+shellQuote(script), bytes.NewReader(data), io.Discard)
}
