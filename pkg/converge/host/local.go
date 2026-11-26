//go:build unix

package host

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/roots/trellis-cli/pkg/converge/state"
)

// Local is the Host the agent uses on the server itself.
type Local struct {
	HTTP *http.Client

	mu     sync.Mutex
	users  map[uint32]string
	groups map[uint32]string
	uids   map[string]int
	gids   map[string]int
}

func NewLocal() *Local {
	return &Local{
		// Keep a connection per parallel download (package prefetch) open:
		// reconnecting per file costs far more than the transfer on a
		// distant mirror.
		HTTP: &http.Client{Timeout: 2 * time.Minute, Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment, MaxIdleConnsPerHost: 8, IdleConnTimeout: 30 * time.Second,
		}},
		users:  map[uint32]string{},
		groups: map[uint32]string{},
		uids:   map[string]int{},
		gids:   map[string]int{},
	}
}

func (l *Local) Stat(path string, hash bool) (state.FileInfo, error) {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return state.FileInfo{}, nil
	}
	if err != nil {
		return state.FileInfo{}, err
	}
	info := state.FileInfo{Exists: true, IsDir: fi.IsDir(), Mode: fi.Mode().Perm()}
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		info.Owner = l.userName(st.Uid)
		info.Group = l.groupName(st.Gid)
	}
	if hash && fi.Mode().IsRegular() {
		f, err := os.Open(path)
		if err != nil {
			return info, err
		}
		defer func() { _ = f.Close() }()
		h := sha256.New()
		if _, err := io.Copy(h, f); err != nil {
			return info, err
		}
		info.SHA256 = hex.EncodeToString(h.Sum(nil))
	}
	return info, nil
}

func (l *Local) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (l *Local) WriteFile(path string, data []byte, mode fs.FileMode, owner, group string, validate func(string) error) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	uid, err := l.uid(owner)
	if err != nil {
		return err
	}
	gid, err := l.gid(group)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".trellis-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Chown before chmod: chown can clear setuid bits.
	if err := os.Chown(tmp, uid, gid); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if validate != nil {
		if err := validate(tmp); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

func (l *Local) Remove(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (l *Local) Run(ctx context.Context, c Cmd) ([]byte, error) {
	if len(c.Argv) == 0 {
		return nil, errors.New("empty command")
	}
	cmd := exec.CommandContext(ctx, c.Argv[0], c.Argv[1:]...)
	cmd.Dir = c.Dir
	if c.Stdin != nil {
		cmd.Stdin = bytes.NewReader(c.Stdin)
	}
	cmd.Env = append(os.Environ(), "LC_ALL=C", "DEBIAN_FRONTEND=noninteractive")
	cmd.Env = append(cmd.Env, c.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("%s: %w: %s", strings.Join(c.Argv, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (l *Local) Fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := l.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 256<<20))
}

func (l *Local) userName(uid uint32) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n, ok := l.users[uid]; ok {
		return n
	}
	n := strconv.Itoa(int(uid))
	if u, err := user.LookupId(n); err == nil {
		n = u.Username
	}
	l.users[uid] = n
	return n
}

func (l *Local) groupName(gid uint32) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n, ok := l.groups[gid]; ok {
		return n
	}
	n := strconv.Itoa(int(gid))
	if g, err := user.LookupGroupId(n); err == nil {
		n = g.Name
	}
	l.groups[gid] = n
	return n
}

func (l *Local) uid(name string) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if id, ok := l.uids[name]; ok {
		return id, nil
	}
	u, err := user.Lookup(name)
	if err != nil {
		return 0, fmt.Errorf("user %q: %w", name, err)
	}
	id, _ := strconv.Atoi(u.Uid)
	l.uids[name] = id
	return id, nil
}

func (l *Local) gid(name string) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if id, ok := l.gids[name]; ok {
		return id, nil
	}
	g, err := user.LookupGroup(name)
	if err != nil {
		return 0, fmt.Errorf("group %q: %w", name, err)
	}
	id, _ := strconv.Atoi(g.Gid)
	l.gids[name] = id
	return id, nil
}
