package lima

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

func TestNewManager(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()

	os.OpenFile(filepath.Join(tmp, "limactl"), os.O_CREATE, 0555)
	path := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("PATH=%s:%s", path, tmp))

	commands := []command.MockCommand{
		{
			Command: "sw_vers",
			Args:    []string{"-productVersion"},
			Output:  `13.0.1`,
		},
		{
			Command: "limactl",
			Args:    []string{"-v"},
			Output:  `limactl version 0.14.2-6-g3b5529f`,
		},
	}
	defer command.MockExecCommands(t, commands)()

	_, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewManagerUnsupportedOS(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()

	os.OpenFile(filepath.Join(tmp, "limactl"), os.O_CREATE, 0555)
	path := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("PATH=%s:%s", path, tmp))

	commands := []command.MockCommand{
		{
			Command: "sw_vers",
			Args:    []string{"-productVersion"},
			Output:  `12.0.1`,
		},
	}
	defer command.MockExecCommands(t, commands)()

	_, err := NewManager(trellis, cli.NewMockUi())
	if err == nil {
		t.Fatal(err)
	}

	expected := "Unsupported OS or macOS version. The macOS Virtualization Framework requires macOS 13.0 (Ventura) or later."

	if err.Error() != expected {
		t.Errorf("expected error to be %q, got %q", expected, err.Error())
	}
}

func TestInitInstance(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TRELLIS_BYPASS_LIMA_REQUIREMENTS", "1")

	manager, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}

	instance := Instance{Name: "test"}
	manager.initInstance(&instance)

	if instance.Name != "test" {
		t.Errorf("expected instance name to be %q, got %q", "test", instance.Name)
	}
}

func TestInstances(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TRELLIS_BYPASS_LIMA_REQUIREMENTS", "1")

	instanceName := "test"

	instancesJson := fmt.Sprintf(`[{"name":"%s","status":"Running","dir":"/foo/test","vmType":"vz","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":60720,"hostAgentPID":9390,"driverPID":9390}]`, instanceName)

	commands := []command.MockCommand{
		{
			Command: "limactl",
			Args:    []string{"inspect"},
			Output:  instancesJson,
		},
	}

	defer command.MockExecCommands(t, commands)()

	manager, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}

	instances := manager.Instances()

	if len(instances) != 1 {
		t.Errorf("expected 1 instance, got %d", len(instances))
	}

	instance, ok := instances[instanceName]

	if !ok {
		t.Errorf("expected instance with name %s to be present", instanceName)
	}

	if instance.Name != instanceName {
		t.Errorf("expected instance name to be %q, got %q", instanceName, instance.Name)
	}
}
