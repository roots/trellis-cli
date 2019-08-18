package github

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mholt/archiver"
)

type Release struct {
	Version string `json:"tag_name"`
	ZipUrl  string `json:"zipball_url"`
}

func DownloadLatestRelease(repo string, path string, dest string) string {
	release := FetchLatestRelease(repo)

	os.Chdir(path)
	archivePath := fmt.Sprintf("%s.zip", release.Version)

	err := DownloadFile(archivePath, release.ZipUrl)
	defer os.Remove(archivePath)

	if err != nil {
		log.Fatal(err)
	}

	if err := archiver.Unarchive(archivePath, path); err != nil {
		log.Fatal(err)
	}

	org := strings.Split(repo, "/")[0]
	dirs, _ := filepath.Glob(fmt.Sprintf("%s-*", org))

	if len(dirs) == 0 {
		log.Fatalln("Error: extracted release zip did not contain the expected directory")
	}

	for _, dir := range dirs {
		err := os.Rename(dir, dest)

		if err != nil {
			os.RemoveAll(dir)
			log.Fatal(err)
		}
	}

	return release.Version
}

func FetchLatestRelease(repo string) Release {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	resp, err := http.Get(url)

	if err != nil {
		log.Fatal(err)
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	release := Release{}

	if err = json.Unmarshal(body, &release); err != nil {
		log.Fatal(err)
	}

	return release
}

func DownloadFile(filepath string, url string) error {
	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
