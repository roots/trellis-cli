// Command trellis-agent runs on a server. `trellis converge plan/apply`
// uploads it over SSH and sends it one request on stdin; it streams the
// plan, events and result back as JSON lines on stdout.
//
// It is deliberately small and separate from trellis-cli: it links only the
// engine (no config parsing, templates or secrets), so the upload is a few
// MB and the server never holds anything but the compiled manifest.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/roots/trellis-cli/pkg/converge/agent"
	"github.com/roots/trellis-cli/pkg/converge/host"
)

func main() {
	if len(os.Args) > 1 {
		fmt.Fprintf(os.Stderr, "trellis-agent (protocol %d) takes no arguments; it is run by `trellis converge`\n", agent.Protocol)
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	// Errors are reported to the client in the stream; the exit status
	// just signals failure.
	if err := agent.Serve(ctx, host.NewLocal(), agent.FileLock, os.Stdin, os.Stdout); err != nil {
		os.Exit(1)
	}
}
