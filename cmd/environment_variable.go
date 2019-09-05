package cmd

import (
	"os"
	"os/exec"
)

func appendEnvironmentVariable(cmd *exec.Cmd, elem string) {
	env := os.Environ()
	// To allow mockExecCommand injects its environment variables
	if cmd.Env != nil {
		env = cmd.Env
	}
	cmd.Env = append(env, elem)
}
