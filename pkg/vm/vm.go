package vm

import (
	"errors"
)

var (
	VmNotFoundErr = errors.New("vm does not exist")
)

type Manager interface {
	CreateInstance(name string) error
	DeleteInstance(name string) error
	InventoryPath() string
	StartInstance(name string) error
	StopInstance(name string) error
	OpenShell(name string, dir string, commandArgs []string) error
}
