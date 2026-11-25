package remote

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// LimaVM is how to reach a local Lima VM over SSH.
type LimaVM struct {
	Port int
	User string // Lima's default user, which has passwordless sudo
	Key  string // Lima's SSH identity
}

// Options are the ssh options for connecting to the VM. Host keys aren't
// recorded: Lima VMs come and go on reused ports.
func (v LimaVM) Options() []string {
	return []string{
		"IdentityFile=" + v.Key,
		"IdentitiesOnly=yes",
		"UserKnownHostsFile=/dev/null",
		"StrictHostKeyChecking=no",
		"LogLevel=ERROR",
	}
}

// Lima looks up a running Lima VM by name.
func Lima(ctx context.Context, x Exec, name string) (LimaVM, error) {
	var out, errb bytes.Buffer
	if err := x.Run(ctx, []string{"limactl", "list", name, "--format", "{{.Status}} {{.SSHLocalPort}} {{.Dir}}"}, nil, &out, &errb); err != nil {
		return LimaVM{}, fmt.Errorf("limactl list %s: %w: %s", name, err, strings.TrimSpace(errb.String()))
	}
	f := strings.Fields(out.String())
	if len(f) != 3 {
		return LimaVM{}, fmt.Errorf("no Lima VM named %q (create one with `limactl start --name=%s template://ubuntu-24.04`)", name, name)
	}
	if f[0] != "Running" {
		return LimaVM{}, fmt.Errorf("Lima VM %s is %s (start it with `limactl start %s`)", name, strings.ToLower(f[0]), name)
	}
	port, err := strconv.Atoi(f[1])
	if err != nil || port == 0 {
		return LimaVM{}, fmt.Errorf("Lima VM %s has no SSH port", name)
	}
	out.Reset()
	if err := x.Run(ctx, []string{"limactl", "shell", name, "whoami"}, nil, &out, &errb); err != nil {
		return LimaVM{}, fmt.Errorf("limactl shell %s whoami: %w", name, err)
	}
	// Instances live in $LIMA_HOME/<name>; the key in $LIMA_HOME/_config.
	key := filepath.Join(filepath.Dir(f[2]), "_config", "user")
	return LimaVM{Port: port, User: strings.TrimSpace(out.String()), Key: key}, nil
}
