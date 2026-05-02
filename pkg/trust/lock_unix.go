//go:build !windows

package trust

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/roots/trellis-cli/app_paths"
)

// AcquireLock takes an exclusive flock on a sentinel file under the trust
// state dir and returns a release function. Callers must defer the release.
//
// This serializes concurrent `trellis vm trust` / `trellis vm untrust`
// invocations so they can't race the state file or the Linux user CA
// bundle. The lock is process-scoped via flock(2), which is supported on
// macOS and Linux — the only platforms this package builds for.
//
// If another process already holds the lock, a one-line note is written to
// notify so the user understands the wait, then the call blocks until the
// lock is acquired.
func AcquireLock(notify io.Writer) (release func(), err error) {
	dir := filepath.Join(app_paths.DataDir(), "state")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	lockPath := filepath.Join(dir, "trust.lock")

	f, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", lockPath, err)
	}

	// Try non-blocking first so we can announce the wait if contended.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			if notify != nil {
				_, _ = fmt.Fprintln(notify, "Waiting for trust lock (another `trellis vm trust` or `vm untrust` is running)...")
			}
			if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
				_ = f.Close()
				return nil, fmt.Errorf("acquire lock %s: %w", lockPath, err)
			}
		} else {
			_ = f.Close()
			return nil, fmt.Errorf("acquire lock %s: %w", lockPath, err)
		}
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
