// Package agentbin embeds the linux trellis-agent executables, so every
// trellis-cli carries agents built from the same source (and protocol).
//
// They are generated, not committed: run `go generate ./pkg/converge/agentbin`
// (or `make agents`) before building a release. Builds without them still
// work; `trellis converge` then needs --agent-binary or $TRELLIS_AGENT_DIR.
package agentbin

//go:generate go run ./gen

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed all:files
var files embed.FS

// Name is the file name of the agent for arch.
func Name(arch string) string { return "trellis-agent-linux-" + arch }

// Agent is an embedded, gzip-compressed agent executable.
type Agent struct {
	Gzip   []byte
	SHA256 string // of the uncompressed executable
}

// Get returns the embedded agent for arch.
func Get(arch string) (Agent, bool) {
	gz, err := fs.ReadFile(files, "files/"+Name(arch)+".gz")
	if err != nil {
		return Agent{}, false
	}
	sum, err := fs.ReadFile(files, "files/"+Name(arch)+".sha256")
	if err != nil {
		return Agent{}, false
	}
	return Agent{Gzip: gz, SHA256: strings.TrimSpace(string(sum))}, true
}
