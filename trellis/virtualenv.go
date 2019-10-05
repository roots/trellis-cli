package trellis

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"trellis-cli/github"
)

const VirtualenvDir string = "virtualenv"
const EnvName string = "VIRTUAL_ENV"

type Virtualenv struct {
	Path       string
	BinPath    string
	OldPathEnv string
}

func NewVirtualenv(path string) *Virtualenv {
	return &Virtualenv{
		Path:    filepath.Join(path, VirtualenvDir),
		BinPath: filepath.Join(path, VirtualenvDir, "bin"),
	}
}

func (v *Virtualenv) Activate() {
	v.OldPathEnv = os.Getenv("PATH")
	os.Setenv(EnvName, v.Path)
	os.Setenv("PATH", fmt.Sprintf("%s:%s", v.BinPath, v.OldPathEnv))
}

func (v *Virtualenv) Active() bool {
	return os.Getenv(EnvName) == v.Path
}

func (v *Virtualenv) Create() (err error) {
	_, cmd := v.Installed()
	cmd.Args = append(cmd.Args, v.Path)

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
	os.Unsetenv(EnvName)
	os.Setenv("PATH", v.OldPathEnv)
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

	return github.DownloadLatestRelease("pypa/virtualenv", os.TempDir(), localPath)
}

func (v *Virtualenv) Installed() (ok bool, cmd *exec.Cmd) {
	path, err := exec.LookPath("virtualenv")
	if err == nil {
		return true, exec.Command(path)
	}

	path, err = exec.LookPath("python3")
	if err == nil {
		return true, exec.Command(path, "-m", "venv")
	}

	localVenvPath := filepath.Join(v.LocalPath(), "virtualenv.py")

	if _, err = os.Stat(localVenvPath); !os.IsNotExist(err) {
		return true, exec.Command("python", localVenvPath)
	}

	return false, nil
}
