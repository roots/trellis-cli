package trellis

import (
	"os"
	"path/filepath"
	"testing"
)

const testDir = "tmp"

func TestDetect(t *testing.T) {
	testDir := t.TempDir()

	devDir := filepath.Join(testDir, "group_vars", "development")

	if os.MkdirAll(devDir, 0700) != nil {
		panic("Unable to create directory")
	}

	devConfig := filepath.Join(devDir, "wordpress_sites.yml")

	if err := os.WriteFile(devConfig, []byte{}, 0666); err != nil {
		t.Fatal(err)
	}

	project := &ProjectDetector{}

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

func TestDetectTrellisProjectStructure(t *testing.T) {
	testDir := t.TempDir()

	trellisDir := filepath.Join(testDir, "trellis")
	siteDir := filepath.Join(testDir, "site")

	os.Mkdir(trellisDir, 0700)
	os.Mkdir(siteDir, 0700)

	os.Mkdir(filepath.Join(trellisDir, ConfigDir), 0700)

	devDir := filepath.Join(trellisDir, "group_vars", "development")
	os.MkdirAll(devDir, 0700)

	devConfig := filepath.Join(devDir, "wordpress_sites.yml")
	os.WriteFile(devConfig, []byte{}, 0666)

	project := &ProjectDetector{}

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
			trellisDir,
		},
		{
			"detects_project_in_trellis_dir",
			trellisDir,
			true,
			trellisDir,
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
