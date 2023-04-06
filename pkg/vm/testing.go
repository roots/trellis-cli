package vm

import (
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

type MockVmManager struct {
	ui      cli.Ui
	trellis *trellis.Trellis
}

func NewMockManager(trellis *trellis.Trellis, ui cli.Ui) (manager *MockVmManager, err error) {
	manager = &MockVmManager{
		trellis: trellis,
		ui:      ui,
	}

	return manager, nil
}

func (m *MockVmManager) CreateVM(name string) error {
	return nil
}

func (m *MockVmManager) DeleteVM(name string) error {
	return nil
}

func (m *MockVmManager) InventoryPath() string {
	return ""
}

func (m *MockVmManager) StartVM(name string) error {
	return nil
}

func (m *MockVmManager) StopVM(name string) error {
	return nil
}

func (m *MockVmManager) OpenShell(name string, commandArgs []string) error {
	return nil
}
