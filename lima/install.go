package lima

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/roots/trellis-cli/github"
)

func Install(installPath string) error {
	tempDir, _ := ioutil.TempDir("", "trellis-lima")
	defer os.RemoveAll(tempDir)

	pattern := fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH)

	github.DownloadAsset(
		"lima-vm/lima",
		"latest",
		tempDir,
		tempDir,
		pattern,
	)

	path := filepath.Join(tempDir, "bin")
	files, err := os.ReadDir(path)
	if err != nil {
		return err
	}

	for _, file := range files {
		err := os.Rename(filepath.Join(path, file.Name()), filepath.Join(installPath, file.Name()))
		if err != nil {
			return err
		}
	}

	sharePath := filepath.Join(tempDir, "share")
	err = os.Rename(sharePath, filepath.Join(installPath, "share"))
	if err != nil {
		return err
	}

	return nil
}
