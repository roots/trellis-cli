package cmd

import (
	"os/exec"
	"syscall"
)

type CommandExecutor interface {
	Exec(argv0 string, argv []string, envv []string) (err error)
	LookPath(file string) (string, error)
}

type SyscallCommandExecutor struct{}

func (s *SyscallCommandExecutor) Exec(argv0 string, argv []string, envv []string) (err error) {
	return syscall.Exec(argv0, argv, envv)
}

func (s *SyscallCommandExecutor) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}
