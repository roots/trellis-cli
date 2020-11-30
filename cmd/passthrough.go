/*
Copyright 2014 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

type PassthroughCommand struct {
	Bin  string
	Name string
	Args []string
}

// Taken from https://github.com/kubernetes/kubectl/blob/b155278f1f4a21a0be2d4f6f0037258dee4d1a22/pkg/cmd/cmd.go#L371
func (c *PassthroughCommand) Run(args []string) int {
	// Windows does not support exec syscall.
	if runtime.GOOS == "windows" {
		cmd := exec.Command(c.Bin, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin
		cmd.Env = os.Environ()
		err := cmd.Run()
		if err == nil {
			return 0
		}
		return 1
	}

	// invoke cmd binary relaying the environment and args given
	// append executablePath to cmdArgs, as execve will make first argument the "binary name".
	if err := syscall.Exec(c.Bin, append([]string{c.Bin}, args...), os.Environ()); err != nil {
		return 1
	}

	return 0
}

func (c *PassthroughCommand) Synopsis() string {
	return fmt.Sprintf("Third party plugin: Forward command to %s", filepath.Base(c.Bin))
}

func (c *PassthroughCommand) Help() string {
	requested := strings.Join(c.Args, " ")

	if strings.HasPrefix(requested, c.Name) {
		commandParts := len(strings.Split(c.Name, " "))
		c.Run(c.Args[commandParts:])
	}

	return ""
}
