package vm

import (
	"errors"
)

var (
	VmNotFoundErr = errors.New("vm does not exist")
)

type Manager interface {
	CreateVM(name string) error
	DeleteVM(name string) error
	InventoryPath() string
	StartVM(name string) error
	StopVM(name string) error
	OpenShell(name string, commandArgs []string) error
}
