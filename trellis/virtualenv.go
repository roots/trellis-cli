package trellis

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/roots/trellis-cli/command"
)

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

/*
Updates the shebang lines in pip generated bin files to properly handle
paths with spaces.

Pip does not properly handle paths with spaces in them when creating the bin
scripts:

	#!/path with spaces/bin/python

This is an invalid POSIX path so Python can't execute the script.

As a workaround, this function replaces that invalid shebang with the workaround
that Virtualenv uses itself for the pip binary:

	#!/bin/sh
	'''exec' "/path with spaces/bin/python" "$0" "$@"
	' '''

If this function is called on a BinPath without spaces, it's just a no-op
that doesn't modify any files.
*/
func (v *Virtualenv) UpdateBinShebangs(binGlob string) error {
	if !strings.Contains(v.BinPath, " ") {
		return nil
	}

	binPaths, _ := filepath.Glob(v.BinPath + "/" + binGlob)

	for _, path := range binPaths {
		if err := v.updateFile(path); err != nil {
			return err
		}
	}

	return nil
}

func (v *Virtualenv) updateFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("Could not open file %s: %w", path, err)
	}

	fileInfo, _ := f.Stat()
	permissions := fileInfo.Mode()
	defer f.Close()

	contents, err := v.replaceShebang(f)
	if err != nil {
		return fmt.Errorf("Error replacing shebang in file %s: %w", path, err)
	}

	// overwrite the original bin file with the fixed version
	if err = os.WriteFile(f.Name(), contents.Bytes(), permissions); err != nil {
		return fmt.Errorf("Error writing file %s: %w", path, err)
	}

	return nil
}

func (v *Virtualenv) replaceShebang(r io.Reader) (*bytes.Buffer, error) {
	var contents bytes.Buffer
	sc := bufio.NewScanner(r)
	lineNumber := 1

	for sc.Scan() {
		line := sc.Text()

		// for extra safety, we only want to match on the first line in the file
		if lineNumber == 1 && strings.HasPrefix(line, "#!"+v.BinPath) {
			shebang := fmt.Sprintf("#!/bin/sh\n'''exec' \"%s/python\" \"$0\" \"$@\"\n' '''", v.BinPath)
			// write new shebang lines to tmp file
			if _, err := contents.WriteString(shebang + "\n"); err != nil {
				return nil, err
			}
		} else {
			// write original line to tmp file
			if _, err := contents.WriteString(line + "\n"); err != nil {
				return nil, err
			}
		}

		lineNumber++
	}

	return &contents, nil
}
