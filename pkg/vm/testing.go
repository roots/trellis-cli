package vm

import (
	"os/exec"

	"github.com/hashicorp/cli"
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

func (m *MockVmManager) CreateInstance(name string) error {
	return nil
}

func (m *MockVmManager) DeleteInstance(name string) error {
	return nil
}

func (m *MockVmManager) InventoryPath() string {
	return ""
}

func (m *MockVmManager) StartInstance(name string) error {
	return nil
}

func (m *MockVmManager) StopInstance(name string) error {
	return nil
}

func (m *MockVmManager) OpenShell(name string, dir string, commandArgs []string) error {
	return nil
}

func (m *MockVmManager) RunCommand(args []string, dir string) error {
	return nil
}

func (m *MockVmManager) RunCommandPipe(args []string, dir string) (*exec.Cmd, error) {
	return nil, nil
}
