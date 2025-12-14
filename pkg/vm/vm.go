package vm

import (
	"errors"
	"os/exec"
)

var (
	ErrVmNotFound = errors.New("vm does not exist")
)

type Manager interface {
	CreateInstance(name string) error
	DeleteInstance(name string) error
	InventoryPath() string
	StartInstance(name string) error
	StopInstance(name string) error
	OpenShell(name string, dir string, commandArgs []string) error
	RunCommand(args []string, dir string) error
	RunCommandPipe(args []string, dir string) (*exec.Cmd, error)
}
