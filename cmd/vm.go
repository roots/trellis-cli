package cmd

import (
	"fmt"
	"runtime"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/lima"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

func newVmManager(t *trellis.Trellis, ui cli.Ui) (vm.Manager, error) {
	vmType := t.VmManagerType()

	switch vmType {
	case "lima":
		return lima.NewManager(t, ui)
	case "mock":
		return vm.NewMockManager(t, ui)
	case "":
		return nil, fmt.Errorf("No VM managers are supported on %s yet.", runtime.GOOS)
	}

	return nil, fmt.Errorf("VM manager not found")
}
