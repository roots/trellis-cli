package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
)

func TestIntegrationForceNoPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	bin := os.Getenv("TEST_BINARY")
	if bin == "" {
		t.Error("TEST_BINARY not supplied")
	}
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Error(bin + " not exist")
	}

	tempDir := t.TempDir()

	file, err := os.Create(filepath.Join(tempDir, "trellis-abc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err = os.Chmod(file.Name(), 0111); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mockUi := cli.NewMockUi()

	trellisCommand := command.WithOptions(command.WithUiOutput(mockUi)).Cmd(bin, []string{"--help"})
	trellisCommand.Env = []string{"PATH=" + tempDir + ":$PATH", "TRELLIS_NO_PLUGINS=true"}

	trellisCommand.Run()
	output := mockUi.ErrorWriter.String()

	for _, unexpected := range []string{"Available third party plugin commands are", "abc"} {
		if strings.Contains(output, unexpected) {
			t.Errorf("unexpected output %q to contain %q", output, unexpected)
		}
	}
}
