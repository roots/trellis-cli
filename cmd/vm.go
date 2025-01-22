package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/pkg/lima"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

const VagrantInventoryFilePath string = ".vagrant/provisioners/ansible/inventory/vagrant_ansible_inventory"

func newVmManager(trellis *trellis.Trellis, ui cli.Ui) (manager vm.Manager, err error) {
	switch trellis.CliConfig.Vm.Manager {
	case "auto":
		switch runtime.GOOS {
		case "darwin", "linux":
			return lima.NewManager(trellis, ui)
		default:
			return nil, fmt.Errorf("No VM managers are supported on %s yet.", runtime.GOOS)
		}
	case "lima":
		return lima.NewManager(trellis, ui)
	case "mock":
		return vm.NewMockManager(trellis, ui)
	}
	return nil, fmt.Errorf("VM manager not found")
}

func findDevInventory(trellis *trellis.Trellis, ui cli.Ui) string {
	manager, managerErr := newVmManager(trellis, ui)

	if managerErr == nil {
		_, vmInventoryErr := os.Stat(manager.InventoryPath())
		if vmInventoryErr == nil {
			return manager.InventoryPath()
		}
	}

	if _, vagrantInventoryErr := os.Stat(filepath.Join(trellis.Path, VagrantInventoryFilePath)); vagrantInventoryErr == nil {
		return VagrantInventoryFilePath
	}

	return ""
}
