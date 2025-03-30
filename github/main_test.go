package github

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mholt/archives"
)

func TestNewReleaseFromVersion(t *testing.T) {
	release := NewReleaseFromVersion("roots/trellis-cli", "1.0.0")

	var expectedRelease = &Release{
		Version: "1.0.0",
		ZipUrl:  "https://api.github.com/repos/roots/trellis-cli/zipball/1.0.0",
	}

	if !cmp.Equal(expectedRelease, release) {
		t.Errorf("expected release %s but got %s", expectedRelease, release)
	}
}

func TestDownloadRelease(t *testing.T) {
	tmpDir := t.TempDir()
	os.Chdir(tmpDir)

	var dir = "roots-trellis"

	err := os.Mkdir(dir, os.ModePerm)
	if err != nil {
		t.Error(err)
	}

	_, err = os.Create(filepath.Join(dir, "test_file"))
	if err != nil {
		t.Error(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-type", "application/octet-stream")
		rw.Header().Set("Content-Disposition", "attachment; filename='release.zip'")

		err = createZipFile(filepath.Join(tmpDir, dir), rw)
		if err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	BaseURL = server.URL
	client := server.Client()
	Client = client

	const expectedVersion = "1.0.0"

	destPath := filepath.Join(tmpDir, "test_release_dir")
	release, err := DownloadRelease("roots/trellis", expectedVersion, tmpDir, destPath)

	expectedPath := filepath.Join(tmpDir, "test_release_dir", "test_file")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("expected extracted file %s to exist", expectedPath)
	}

	if expectedVersion != release.Version {
		t.Errorf("expected version %s but got %s", expectedVersion, release.Version)
	}
}

func TestFetechLatestRelease(t *testing.T) {
	cases := []struct {
		name          string
		response      string
		responseError string
		release       *Release
	}{
		{
			"success response",
			fmt.Sprintf(`{
  "tag_name": "v1.0",
  "html_url": "https://github.com/roots/trellis-cli/releases/tag/v1.0",
  "zipball_url": "https://api.github.com/repos/roots/trellis-cli/zipball/v1.0"
}`),
			"",
			&Release{
				Version: "v1.0",
				URL:     "https://github.com/roots/trellis-cli/releases/tag/v1.0",
				ZipUrl:  "https://api.github.com/repos/roots/trellis-cli/zipball/v1.0",
			},
		},
		{
			"error response",
			"",
			"some error",
			nil,
		},
	}

	for _, tc := range cases {
		server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			if tc.responseError != "" {
				http.Error(rw, tc.responseError, 400)
			} else {
				rw.Write([]byte(tc.response))
			}
		}))
		defer server.Close()

		BaseURL = server.URL
		client := server.Client()

		release, _ := FetchLatestRelease("roots/trellis-cli", client)

		if !cmp.Equal(tc.release, release) {
			t.Errorf("expected release %s but got %s", tc.release, release)
		}
	}
}

func createZipFile(dir string, writer io.Writer) error {
	var format archives.Zip
	ctx := context.Background()

	files, err := archives.FilesFromDisk(ctx, nil, map[string]string{
		dir: filepath.Base(dir),
	})

	if err != nil {
		return err
	}

	err = format.Archive(ctx, writer, files)
	if err != nil {
		return err
	}
	return nil
}
