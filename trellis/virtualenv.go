package trellis

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/roots/trellis-cli/github"
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

func (v *Virtualenv) LocalPath() string {
	configHome := os.Getenv("XDG_CONFIG_HOME")

	if configHome == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			log.Fatal(err)
		}

		configHome = filepath.Join(homeDir, ".local", "share")
	}

	return filepath.Join(configHome, "trellis", "virtualenv")
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

func (v *Virtualenv) Install() string {
	localPath := v.LocalPath()
	configDir := filepath.Dir(localPath)

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		if err = os.MkdirAll(configDir, 0755); err != nil {
			log.Fatal(err)
		}
	}

	return github.DownloadRelease("pypa/virtualenv", "latest", os.TempDir(), localPath)
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

	localVenvPath := filepath.Join(v.LocalPath(), "virtualenv.py")

	if _, err = os.Stat(localVenvPath); !os.IsNotExist(err) {
		return true, exec.Command("python", localVenvPath)
	}

	return false, nil
}
