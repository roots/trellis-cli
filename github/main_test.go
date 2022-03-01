package github

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/mholt/archiver"
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
	tmpDir, _ := ioutil.TempDir("", "release_test")
	defer os.RemoveAll(tmpDir)
	os.Chdir(tmpDir)

	var dir = "roots-trellis"

	err := os.Mkdir(dir, os.ModePerm)
	if err != nil {
		t.Error(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-type", "application/octet-stream")
		rw.Header().Set("Content-Disposition", "attachment; filename='release.zip'")

		err = createZipFile([]string{dir}, rw)
		if err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()

	BaseURL = server.URL
	client := server.Client()
	Client = client

	const expectedVersion = "1.0.0"

	release, err := DownloadRelease("roots/trellis", expectedVersion, tmpDir, filepath.Join(tmpDir, "test_release_dir"))

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

func createZipFile(files []string, writer io.Writer) error {
	zip := archiver.NewZip()

	err := zip.Create(writer)
	if err != nil {
		return err
	}

	defer zip.Close()

	for _, fname := range files {
		info, err := os.Stat(fname)
		if err != nil {
			return err
		}

		internalName, err := archiver.NameInArchive(info, fname, fname)
		if err != nil {
			return err
		}

		file, err := os.Open(fname)
		if err != nil {
			return err
		}

		err = zip.Write(archiver.File{
			FileInfo: archiver.FileInfo{
				FileInfo:   info,
				CustomName: internalName,
			},
			ReadCloser: file,
		})

		if err != nil {
			return err
		}
	}

	return nil
}
