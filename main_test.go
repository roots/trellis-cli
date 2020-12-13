package main

import (
	"fmt"
	"github.com/mitchellh/cli"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"github.com/roots/trellis-cli/cmd"
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

	tempDir, err := ioutil.TempDir(os.TempDir(), "test-cmd-plugins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// cleanup
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			panic(fmt.Errorf("unexpected cleanup error: %v", err))
		}
	}()

	file, err := os.Create(filepath.Join(tempDir, "trellis-abc"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err = os.Chmod(file.Name(), 0111); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mockUi := cli.NewMockUi()

	trellisCommand := cmd.CommandExecWithOutput(bin, []string{"--help"}, mockUi)
	trellisCommand.Env = []string{"PATH=" + tempDir + ":$PATH", "TRELLIS_NO_PLUGINS=true"}

	trellisCommand.Run()
	output := mockUi.ErrorWriter.String()

	for _, unexpected := range []string{"Available third party plugin commands are", "abc"} {
		if strings.Contains(output, unexpected) {
			t.Errorf("unexpected output %q to contain %q", output, unexpected)
		}
	}
}
