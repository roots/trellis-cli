package cmd

import (
	"fmt"
	"os"
	"runtime"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/lima"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/pkg/wsl"
	"github.com/roots/trellis-cli/trellis"
)

// wslTerminalRequired checks if the user is on Windows with the WSL backend.
// Ansible-dependent commands must be run from the WSL terminal, not Windows.
// Returns true if the command should abort with a helpful message.
func wslTerminalRequired(t *trellis.Trellis, ui cli.Ui, command string) bool {
	if runtime.GOOS != "windows" || t.VmManagerType() != "wsl" {
		return false
	}

	ui.Warn(color.YellowString("This command requires Ansible, which is installed inside your WSL environment."))
	ui.Warn(color.YellowString(fmt.Sprintf("Run `trellis vm open` to launch VS Code in WSL, then run `trellis %s` from the integrated terminal.", command)))
	return true
}

// windowsHostRequired checks if the user is inside WSL trying to run a
// command that manages the VM from the Windows host side.
// Returns true if the command should abort with a helpful message.
func windowsHostRequired(t *trellis.Trellis, ui cli.Ui, command string) bool {
	if runtime.GOOS != "linux" || os.Getenv("WSL_DISTRO_NAME") == "" {
		return false
	}

	ui.Warn(color.YellowString(fmt.Sprintf("'trellis %s' manages the WSL distro from the Windows host.", command)))
	ui.Warn(color.YellowString("Run this command from your Windows PowerShell or Command Prompt, not from inside WSL."))
	return true
}

func newVmManager(t *trellis.Trellis, ui cli.Ui) (vm.Manager, error) {
	vmType := t.VmManagerType()

	switch vmType {
	case "lima":
		return lima.NewManager(t, ui)
	case "wsl":
		return wsl.NewManager(t, ui)
	case "mock":
		return vm.NewMockManager(t, ui)
	case "":
		return nil, fmt.Errorf("No VM managers are supported on %s yet.", runtime.GOOS)
	}

	return nil, fmt.Errorf("VM manager not found")
}
