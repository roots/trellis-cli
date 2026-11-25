// Command gen builds the linux agents into ../files: a gzip-compressed
// executable and the sha256 of the uncompressed one per architecture.
// Builds are reproducible (-trimpath, no build ID or VCS stamp), so the
// same source produces the same agent and servers don't re-upload it.
package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/roots/trellis-cli/pkg/converge/agentbin"
)

var arches = []string{"amd64", "arm64"}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "agentbin:", err)
		os.Exit(1)
	}
}

func run() error {
	// go generate runs in the package directory.
	dist, err := filepath.Abs("files")
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "trellis-agent-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	for _, arch := range arches {
		exe := filepath.Join(tmp, agentbin.Name(arch))
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=", "-o", exe, "github.com/roots/trellis-cli/cmd/trellis-agent")
		cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+arch, "CGO_ENABLED=0")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", arch, err)
		}
		b, err := os.ReadFile(exe)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		var gz bytes.Buffer
		w, _ := gzip.NewWriterLevel(&gz, gzip.BestCompression)
		if _, err := w.Write(b); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		base := filepath.Join(dist, agentbin.Name(arch))
		if err := os.WriteFile(base+".gz", gz.Bytes(), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(base+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Printf("%s: %.1f MB (%.1f MB gzipped)\n", agentbin.Name(arch), float64(len(b))/1e6, float64(gz.Len())/1e6)
	}
	return nil
}
