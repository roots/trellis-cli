//go:build unix

package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"syscall"

	"github.com/roots/trellis-cli/pkg/converge/state"
)

// FileLock serialises runs on the server. Provisioning (as root) takes the
// state lock and the deploy lock; deploys (as web) take only the deploy
// lock, which they open read-only (flock works on any open file). So a
// deploy never runs during an apply, and two deploys never overlap.
func FileLock(release bool) (func(), error) {
	root := os.Geteuid() == 0
	if release {
		if root {
			return nil, errors.New("deploys run as the web user, not root")
		}
		f, err := os.Open(state.DeployLockPath)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("this server isn't set up for deploys yet: run `trellis converge apply` first")
		}
		if err != nil {
			return nil, err
		}
		return flock(f)
	}
	if !root {
		return nil, errors.New("provisioning must run as root")
	}
	// 0711: web can open the deploy lock by name but can't list or read
	// anything else here.
	if err := os.MkdirAll(state.Dir, 0o711); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(state.LockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	unlock, err := flock(f)
	if err != nil {
		return nil, err
	}
	d, err := os.OpenFile(state.DeployLockPath, os.O_CREATE|os.O_RDONLY, 0o644)
	if err != nil {
		unlock()
		return nil, err
	}
	unlockDeploy, err := flock(d)
	if err != nil {
		unlock()
		return nil, errors.New("a deploy is in progress on this host")
	}
	return func() { unlockDeploy(); unlock() }, nil
}

func flock(f *os.File) (func(), error) {
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
