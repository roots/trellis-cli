package trellis

import (
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"
)

const testDir = "tmp"

func TestDetectWithPaths(t *testing.T) {
	testDir, _ := ioutil.TempDir("", "trellis")
	defer os.RemoveAll(testDir)

	devDir := filepath.Join(testDir, "group_vars", "development")

	if os.MkdirAll(devDir, 0700) != nil {
		panic("Unable to create directory")
	}

	devConfig := filepath.Join(devDir, "wordpress_sites.yml")

	if err := ioutil.WriteFile(devConfig, []byte{}, 0666); err != nil {
		log.Fatal(err)
	}

	project := &Project{}

	cases := []struct {
		name         string
		path         string
		ok           bool
		expectedPath string
	}{
		{
			"detects_project_in_root_dir",
			testDir,
			true,
			testDir,
		},
		{
			"detects_project_in_subdir",
			devDir,
			true,
			testDir,
		},
		{
			"nothing_detected_outside_of_root_dir",
			filepath.Dir(testDir),
			false,
			"",
		},
	}

	for _, tc := range cases {
		detectedPath, ok := project.Detect(tc.path)

		if ok != tc.ok {
			t.Errorf("expected ok to be %t, but got %t", tc.ok, ok)
		}

		if detectedPath != tc.expectedPath {
			t.Errorf("expected path %s but got %s", tc.expectedPath, detectedPath)
		}
	}
}
