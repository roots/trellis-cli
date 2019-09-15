package cmd

import (
	"golang.org/x/sys/unix"
	"os/exec"
)

type CommandExecutor interface {
	Exec(argv0 string, argv []string, envv []string) (err error)
	LookPath(file string) (string, error)
}

type SyscallCommandExecutor struct{}

func (s *SyscallCommandExecutor) Exec(argv0 string, argv []string, envv []string) (err error) {
	return unix.Exec(argv0, argv, envv)
}

func (s *SyscallCommandExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}
