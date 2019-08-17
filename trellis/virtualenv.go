package trellis

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"trellis-cli/github"
)

const VirtualenvDir string = "virtualenv"
const EnvName string = "VIRTUALENV"

type Virtualenv struct {
	Path    string
	BinPath string
}

func NewVirtualenv(path string) *Virtualenv {
	return &Virtualenv{
		Path:    filepath.Join(path, VirtualenvDir),
		BinPath: filepath.Join(path, VirtualenvDir, "bin"),
	}
}

func (v *Virtualenv) Activate() {
	os.Setenv(EnvName, v.Path)
	os.Setenv("PATH", fmt.Sprintf("%s/bin:$PATH", v.Path))
}

func (v *Virtualenv) Active() bool {
	return v.BinPath == os.Getenv(EnvName)
}

func (v *Virtualenv) Create() (err error) {
	// TODO: error if not installed? or install
	_, cmd := v.Installed()

	cmd.Args = append(cmd.Args, v.Path)

	err = cmd.Run()
	if err != nil {
		return err
	}

	v.Activate()
	return nil
}

func (v *Virtualenv) Initialized() bool {
	if _, err := os.Stat(filepath.Join(v.BinPath, "python")); os.IsNotExist(err) {
		return false
	}

	if _, err := os.Stat(filepath.Join(v.BinPath, "pip")); os.IsNotExist(err) {
		return false
	}

	return true
}

func (v *Virtualenv) Install(path string) string {
	installPath := github.DownloadLatestRelease("pypa/virtualenv", os.TempDir(), path)
	return installPath
}

func (v *Virtualenv) Installed() (ok bool, cmd *exec.Cmd) {
	path, err := exec.LookPath("python3")
	if err == nil {
		return true, exec.Command(path, "-m", "venv")
	}

	path, err = exec.LookPath("virtualenv")
	if err == nil {
		return true, exec.Command(path)
	}

	// TODO: check for local virtualenv installed by trellis-cli

	return false, nil
}
