// Package cmd contains all command-line interface commands for Trellis CLI.
package cmd

import (
	"fmt"
	"runtime"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/pkg/lima"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

// This file provides centralized declarations of functions as variables
// to enable mocking in tests and prevent duplicate implementations.
// Previously, these functions were defined in multiple places, causing
// "redeclared in this block" errors during compilation.

// Declare these as variables so they can be overridden in tests
var (
	// newVmManager creates a VM manager instance based on configuration.
	// Using a function variable allows for mocking in tests and centralizes VM manager creation logic.
	// This approach eliminates duplicate implementations that previously existed across files.
	newVmManager = func(t *trellis.Trellis, ui cli.Ui) (vm.Manager, error) {
		// Select appropriate VM manager based on configuration
		switch t.CliConfig.Vm.Manager {
		case "auto":
			switch runtime.GOOS {
			case "darwin":
				return lima.NewManager(t, ui)
			default:
				return nil, fmt.Errorf("no VM managers are supported on %s yet", runtime.GOOS)
			}
		case "lima":
			return lima.NewManager(t, ui)
		case "mock":
			return vm.NewMockManager(t, ui)
		}

		return nil, fmt.Errorf("vm manager not found")
	}

	// NewProvisionCommand creates a new ProvisionCommand instance.
	// This was moved here from provision.go to follow the same pattern
	// of using function variables for testability.
	NewProvisionCommand = func(ui cli.Ui, trellis *trellis.Trellis) *ProvisionCommand {
		return &ProvisionCommand{UI: ui, Trellis: trellis}
	}
)
