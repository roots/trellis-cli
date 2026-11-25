// Package host is the engine's only port to the machine it manages. The
// engine (observe/plan/apply) talks to a Host; production uses Local (the
// agent running as root on the server), tests use hosttest.Fake.
package host

import (
	"context"
	"io/fs"

	"github.com/roots/trellis-cli/pkg/converge/state"
)

// Cmd is a process to run. Secrets go in Stdin or Env, never Argv.
type Cmd struct {
	Argv  []string
	Stdin []byte
	Env   []string
}

// Host is everything the engine needs from a machine.
type Host interface {
	// Stat reports a path; hash computes SHA256 for regular files.
	Stat(path string, hash bool) (state.FileInfo, error)
	ReadFile(path string) ([]byte, error)
	// WriteFile atomically replaces path (creating parent directories as
	// root 0755). If validate is non-nil it is called with the staged
	// file's path before the rename; an error aborts the write.
	WriteFile(path string, data []byte, mode fs.FileMode, owner, group string, validate func(staged string) error) error
	// Remove deletes path; a missing path is not an error.
	Remove(path string) error
	// Run executes a command and returns its stdout. A non-zero exit is an
	// error that includes stderr; stdout is still returned.
	Run(ctx context.Context, cmd Cmd) ([]byte, error)
	// Fetch downloads a URL.
	Fetch(ctx context.Context, url string) ([]byte, error)
}
