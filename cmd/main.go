package cmd

import (
	"fmt"
	"runtime"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/pkg/lima"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

// Declare these as variables so they can be overridden in tests
var (
	newVmManager = func(t *trellis.Trellis, ui cli.Ui) (vm.Manager, error) {
			// Incorporate the logic from the previous vm.go implementation
			switch t.CliConfig.Vm.Manager {
			case "auto":
				switch runtime.GOOS {
				case "darwin":
					return lima.NewManager(t, ui)
				default:
					return nil, fmt.Errorf("No VM managers are supported on %s yet.", runtime.GOOS)
				}
			case "lima":
				return lima.NewManager(t, ui)
			case "mock":
				return vm.NewMockManager(t, ui)
			}

			return nil, fmt.Errorf("VM manager not found")
		}
	
	NewProvisionCommand = func(ui cli.Ui, trellis *trellis.Trellis) *ProvisionCommand {
		return &ProvisionCommand{UI: ui, Trellis: trellis}
	}
)
