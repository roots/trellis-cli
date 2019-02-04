package trellis

import (
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
)

func loadFixtureProject(t *testing.T) func() {
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	tempDir, err := ioutil.TempDir("", "trellis")
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	cmd := exec.Command("cp", "-a", "testdata/trellis/", tempDir)
	err = cmd.Run()

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	os.Chdir(tempDir)

	return func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("err: %s", err)
		}

		os.RemoveAll(tempDir)
	}
}
