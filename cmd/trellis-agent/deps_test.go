package main

import (
	"os/exec"
	"strings"
	"testing"
)

// The agent runs on every server, so it links only the engine. This keeps
// it small and keeps config parsing, templates and secrets off servers.
func TestAgentDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go list")
	}
	allowed := map[string]bool{
		"github.com/roots/trellis-cli/cmd/trellis-agent":     true,
		"github.com/roots/trellis-cli/pkg/converge/agent":    true,
		"github.com/roots/trellis-cli/pkg/converge/engine":   true,
		"github.com/roots/trellis-cli/pkg/converge/host":     true,
		"github.com/roots/trellis-cli/pkg/converge/manifest": true,
		"github.com/roots/trellis-cli/pkg/converge/release":  true,
		"github.com/roots/trellis-cli/pkg/converge/state":    true,
	}
	cmd := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", ".")
	cmd.Env = append(cmd.Environ(), "GOOS=linux", "CGO_ENABLED=0")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range strings.Fields(string(out)) {
		if !allowed[p] {
			t.Errorf("trellis-agent must not depend on %s", p)
		}
	}
}
