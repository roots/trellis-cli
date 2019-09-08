package trellis

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewVirtualenv(t *testing.T) {
	venv := NewVirtualenv("trellis")
	path := "trellis/virtualenv"
	binPath := "trellis/virtualenv/bin"

	if venv.Path != path {
		t.Errorf("expected path to be %s, but got %s", venv.Path, path)
	}

	if venv.BinPath != binPath {
		t.Errorf("expected path to be %s, but got %s", venv.Path, binPath)
	}
}

func TestActivateSetsEnv(t *testing.T) {
	venv := NewVirtualenv("trellis")
	originalPath := os.Getenv("PATH")

	venv.Activate()

	if os.Getenv("VIRTUALENV") != "trellis/virtualenv" {
		t.Error("expected VIRTUALENV env var to set")
	}

	if os.Getenv("PATH") != "trellis/virtualenv/bin:$PATH" {
		t.Error("expected PATH to contain bin path")
	}

	if venv.OldPathEnv != originalPath {
		t.Error("expected OldPathEnv to be the original PATH")
	}

	venv.Deactivate()
}

func TestActive(t *testing.T) {
	venv := NewVirtualenv("trellis")

	if venv.Active() {
		t.Error("expected virtualenv to be inactive")
	}

	venv.Activate()

	if !venv.Active() {
		t.Error("expected virtualenv to be active")
	}
}

func TestDeactive(t *testing.T) {
	venv := NewVirtualenv("trellis")
	venv.Activate()
	venv.Deactivate()

	if os.Getenv("VIRTUALENV") != "" {
		t.Error("Expected VIRTUALENV to be empty")
	}

	if os.Getenv("PATH") != venv.OldPathEnv {
		t.Error("Expected PATH to be reset")
	}
}

func TestLocalPath(t *testing.T) {
	venv := NewVirtualenv("trellis")
	originalConfigHome := os.Getenv("XDG_CONFIG_HOME")
	os.Unsetenv("XDG_CONFIG_HOME")
	defer os.Setenv("XDG_CONFIG_HOME", originalConfigHome)

	homeDir, _ := os.UserHomeDir()

	localPath := venv.LocalPath()

	if localPath != filepath.Join(homeDir, ".local/share/trellis/virtualenv") {
		t.Error("Expected LocalPath to default to $USER/.local/share")
	}

	os.Setenv("XDG_CONFIG_HOME", "mydir")
	defer os.Setenv("XDG_CONFIG_HOME", originalConfigHome)

	localPath = venv.LocalPath()

	if localPath != filepath.Join("mydir", "trellis/virtualenv") {
		t.Error("Expected LocalPath to use XDG_CONFIG_HOME when set")
	}
}

func TestInitialized(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "trellis")
	defer os.RemoveAll(tempDir)

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	venv := NewVirtualenv(tempDir)

	if venv.Initialized() {
		t.Error("Expected to be uniniatlized")
	}

	os.MkdirAll(venv.BinPath, os.ModePerm)
	testCreateFile(t, filepath.Join(venv.BinPath, "python"))()
	testCreateFile(t, filepath.Join(venv.BinPath, "pip"))()

	if !venv.Initialized() {
		t.Error("Expected to be initialized")
	}
}

func TestInstalled(t *testing.T) {
	defer testSetEnv("PATH", "")()
	defer testSetEnv("XDG_CONFIG_HOME", "none")()

	venv := NewVirtualenv("foo")

	ok, _ := venv.Installed()

	if ok {
		t.Error("Expected to be uninstalled")
	}
}

func TestInstalledPython3(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "trellis")
	defer os.RemoveAll(tempDir)

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	defer testSetEnv("PATH", tempDir)()

	pythonPath := filepath.Join(tempDir, "python3")
	os.OpenFile(pythonPath, os.O_CREATE, 0555)

	venv := NewVirtualenv(tempDir)

	ok, cmd := venv.Installed()

	if !ok {
		t.Error("Expected to be installed")
	}

	if strings.Join(cmd.Args, " ") != fmt.Sprintf("%s -m venv", pythonPath) {
		t.Error("Expected args incorrect")
	}
}

func TestInstalledVirtualenv(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "trellis")
	defer os.RemoveAll(tempDir)

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	defer testSetEnv("PATH", tempDir)()

	venvPath := filepath.Join(tempDir, "virtualenv")
	os.OpenFile(venvPath, os.O_CREATE, 0555)

	venv := NewVirtualenv(tempDir)

	ok, cmd := venv.Installed()

	if !ok {
		t.Error("Expected to be installed")
	}

	if strings.Join(cmd.Args, " ") != venvPath {
		t.Error("Expected args incorrect")
	}
}

func TestInstalledLocalVirtualenv(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "trellis")
	defer os.RemoveAll(tempDir)

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	defer testSetEnv("PATH", "")()
	defer testSetEnv("XDG_CONFIG_HOME", tempDir)()

	venv := NewVirtualenv(tempDir)
	localVenvPath := filepath.Join(venv.LocalPath(), "virtualenv.py")
	os.MkdirAll(venv.LocalPath(), os.ModePerm)
	testCreateFile(t, localVenvPath)()

	ok, cmd := venv.Installed()

	if !ok {
		t.Error("Expected to be installed")
	}

	if strings.Join(cmd.Args, " ") != fmt.Sprintf("python %s", localVenvPath) {
		t.Error("Expected args incorrect")
	}
}

func testCreateFile(t *testing.T, path string) func() {
	file, err := os.Create(path)

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	return func() { file.Close() }
}

func testSetEnv(env string, value string) func() {
	old := os.Getenv(env)
	os.Setenv(env, value)
	return func() { os.Setenv(env, old) }
}
