//go:build windows

package trust

import "io"

// AcquireLock is a no-op on Windows. The trust feature isn't supported
// there (Default() returns an error before any lock would be needed),
// but the package must still compile for cross-builds.
func AcquireLock(_ io.Writer) (release func(), err error) {
	return func() {}, nil
}
