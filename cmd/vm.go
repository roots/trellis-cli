package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/lima"
	"github.com/roots/trellis-cli/pkg/lxd"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

func newVmManager(trellis *trellis.Trellis, ui cli.Ui) (manager vm.Manager, err error) {
	switch trellis.CliConfig.Vm.Manager {
	case "auto":
		switch runtime.GOOS {
		case "darwin":
			return lima.NewManager(trellis, ui)
		case "linux":
			return lxd.NewManager(trellis, ui)
		default:
			return nil, fmt.Errorf("No VM managers are supported on %s yet.", runtime.GOOS)
		}
	case "lima":
		return lima.NewManager(trellis, ui)
	case "lxd":
		return lxd.NewManager(trellis, ui)
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

	return ""
}
