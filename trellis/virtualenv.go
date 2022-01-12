package trellis

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/roots/trellis-cli/command"
)

const TrellisVenvEnvName string = "TRELLIS_VENV"
const VenvEnvName string = "VIRTUAL_ENV"
const PathEnvName string = "PATH"
const OldPathEnvName string = "PRE_TRELLIS_PATH"
const VirtualenvDir string = "virtualenv"

type Virtualenv struct {
	Path    string
	BinPath string
	OldPath string
}

func NewVirtualenv(path string) *Virtualenv {
	return &Virtualenv{
		Path:    filepath.Join(path, VirtualenvDir),
		BinPath: filepath.Join(path, VirtualenvDir, "bin"),
		OldPath: os.Getenv(PathEnvName),
	}
}

func (v *Virtualenv) Activate() {
	if v.Active() {
		return
	}

	os.Setenv(VenvEnvName, v.Path)
	os.Setenv(OldPathEnvName, v.OldPath)
	os.Setenv(PathEnvName, fmt.Sprintf("%s:%s", v.BinPath, v.OldPath))
}

func (v *Virtualenv) Active() bool {
	return os.Getenv(VenvEnvName) == v.Path
}

func (v *Virtualenv) Create() (err error) {
	_, cmd := v.Installed()
	cmd.Args = append(cmd.Args, v.Path)
	cmd.Stderr = os.Stderr

	if v.Initialized() {
		v.Activate()
		return nil
	}

	err = cmd.Run()
	if err != nil {
		return err
	}

	v.Activate()
	return nil
}

func (v *Virtualenv) Deactivate() {
	os.Unsetenv(VenvEnvName)
	os.Unsetenv(OldPathEnvName)
	os.Setenv(PathEnvName, v.OldPath)
}

func (v *Virtualenv) Delete() error {
	return os.RemoveAll(v.Path)
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

func (v *Virtualenv) Installed() (ok bool, cmd *exec.Cmd) {
	path, err := exec.LookPath("python3")
	if err == nil {
		err = command.Cmd(path, []string{"-m", "ensurepip", "--version"}).Run()

		if err == nil {
			return true, command.Cmd(path, []string{"-m", "venv"})
		}
	}

	path, err = exec.LookPath("virtualenv")
	if err == nil {
		return true, command.Cmd(path, []string{})
	}

	return false, nil
}
