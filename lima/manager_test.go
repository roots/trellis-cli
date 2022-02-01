package lima

import (
	"fmt"
	"testing"

	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

func TestNewManager(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	commands := []command.MockCommand{
		{
			Command: "sw_vers",
			Args:    []string{"-productVersion"},
			Output:  `13.0.1`,
		},
	}
	defer command.MockExecCommands(t, commands)()

	_, err := NewManager(trellis)
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

	commands := []command.MockCommand{
		{
			Command: "sw_vers",
			Args:    []string{"-productVersion"},
			Output:  `12.0.1`,
		},
	}
	defer command.MockExecCommands(t, commands)()

	_, err := NewManager(trellis)
	if err == nil {
		t.Fatal(err)
	}

	expected := "Unsupported OS or macOS version. The macOS Virtualization Framework requires macOS 13.0 (Ventura) or later."

	if err.Error() != expected {
		t.Errorf("expected error to be %q, got %q", expected, err.Error())
	}
}

func TestNewInstance(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	commands := []command.MockCommand{
		{
			Command: "sw_vers",
			Args:    []string{"-productVersion"},
			Output:  `13.0.1`,
		},
	}
	defer command.MockExecCommands(t, commands)()

	manager, err := NewManager(trellis)
	if err != nil {
		t.Fatal(err)
	}

	instance := manager.NewInstance("test")

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

	instanceName := "test"

	instancesJson := fmt.Sprintf(`{"name":"%s","status":"Running","dir":"/foo/test","vmType":"vz","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":60720,"hostAgentPID":9390,"driverPID":9390}`, instanceName)

	commands := []command.MockCommand{
		{
			Command: "sw_vers",
			Args:    []string{"-productVersion"},
			Output:  `13.0.1`,
		},
		{
			Command: "limactl",
			Args:    []string{"list", "--json"},
			Output:  instancesJson,
		},
	}

	defer command.MockExecCommands(t, commands)()

	manager, err := NewManager(trellis)
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
