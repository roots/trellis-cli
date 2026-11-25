//go:build unix

package agent

import (
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/roots/trellis-cli/pkg/converge/state"
)

// FileLock takes an exclusive, non-blocking flock on the state lock file.
func FileLock() (func(), error) {
	if err := os.MkdirAll(state.Dir, 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(state.LockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("another trellis run is in progress on this host")
		}
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
