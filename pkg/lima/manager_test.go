package lima

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type MockHostsResolver struct {
	Hosts map[string]string
}

func TestNewManager(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()

	_, _ = os.OpenFile(filepath.Join(tmp, "limactl"), os.O_CREATE, 0555)
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
			Output:  `limactl version 0.15.0`,
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

	_, _ = os.OpenFile(filepath.Join(tmp, "limactl"), os.O_CREATE, 0555)
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

	expected := "unsupported OS or macOS version. The macOS Virtualization Framework requires macOS 13.0 (Ventura) or later."

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

func TestNewInstanceUbuntuVersion(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	trellis.CliConfig.Vm.Ubuntu = "20.04"

	t.Setenv("TRELLIS_BYPASS_LIMA_REQUIREMENTS", "1")

	manager, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}

	instance := manager.newInstance("test")

	if instance.Name != "test" {
		t.Errorf("expected instance name to be %q, got %q", "test", instance.Name)
	}

	if len(instance.Config.Images) != 2 {
		t.Errorf("expected instance config to have 2 images, got %d", len(instance.Config.Images))
	}

	if instance.Config.Images[0].Alias != "focal" {
		t.Errorf("expected instance config to have focal image, got %q", instance.Config.Images[0].Alias)
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

	instancesJson := fmt.Sprintf(`{"name":"%s","status":"Running","dir":"/foo/test","vmType":"vz","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":60720,"hostAgentPID":9390,"driverPID":9390}
{"name":"test2","status":"Running","dir":"/foo/test","vmType":"vz","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":60720,"hostAgentPID":9390,"driverPID":9390}`, instanceName)

	commands := []command.MockCommand{
		{
			Command: "limactl",
			Args:    []string{"ls", "--format=json"},
			Output:  instancesJson,
		},
	}

	defer command.MockExecCommands(t, commands)()

	manager, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}

	instances := manager.instances()

	if len(instances) != 2 {
		t.Errorf("expected 2 instance, got %d", len(instances))
	}

	instance, ok := instances[instanceName]

	if !ok {
		t.Errorf("expected instance with name %s to be present", instanceName)
	}

	if instance.Name != instanceName {
		t.Errorf("expected instance name to be %q, got %q", instanceName, instance.Name)
	}
}

func TestCreateInstance(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TRELLIS_BYPASS_LIMA_REQUIREMENTS", "1")

	ui := cli.NewMockUi()
	manager, err := NewManager(trellis, ui)
	if err != nil {
		t.Fatal(err)
	}

	hostsStorage := make(map[string]string)
	manager.HostsResolver = &MockHostsResolver{Hosts: hostsStorage}

	instanceName := "test"
	sshPort := 60720
	username := "user1"
	ip := "192.168.64.2"

	commands := []command.MockCommand{
		{
			Command: "limactl",
			Args:    []string{"create", "--tty=false", "--name=" + instanceName, "-"},
			Output:  ``,
		},
		{
			Command: "limactl",
			Args:    []string{"shell", instanceName, "whoami"},
			Output:  username,
		},
		{
			Command: "limactl",
			Args:    []string{"ls", "--format=json"},
			Output:  fmt.Sprintf(`{"name":"%s","status":"Stopped","dir":"/foo/test","vmType":"vz","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":%d,"hostAgentPID":9390,"driverPID":9390}`, instanceName, sshPort),
		},
		{
			Command: "limactl",
			Args:    []string{"shell", "--workdir", "/", instanceName, "ip", "route", "show", "dev", "lima0"},
			Output: fmt.Sprintf(`default via 192.168.64.1 proto dhcp src %s metric 100
192.168.64.0/24 proto kernel scope link src 192.168.64.2
192.168.64.1 proto dhcp scope link src 192.168.64.2 metric 100
`, ip),
		},
	}

	defer command.MockExecCommands(t, commands)()

	if err = manager.CreateInstance(instanceName); err != nil {
		t.Fatal(err)
	}

	_, ok := manager.GetInstance(instanceName)

	if !ok {
		t.Errorf("expected instance to be found")
	}
}

func TestStartInstance(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()
	if err := trellis.LoadProject(); err != nil {
		t.Fatal(err)
	}

	t.Setenv("TRELLIS_BYPASS_LIMA_REQUIREMENTS", "1")

	ui := cli.NewMockUi()
	manager, err := NewManager(trellis, ui)
	if err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()

	hostsStorage := make(map[string]string)
	manager.HostsResolver = &MockHostsResolver{Hosts: hostsStorage}

	instanceName := "test"
	sshPort := 60720
	username := "user1"
	ip := "192.168.64.2"

	commands := []command.MockCommand{
		{
			Command: "limactl",
			Args:    []string{"create", "--tty=false", "--name=" + instanceName, "-"},
			Output:  ``,
		},
		{
			Command: "limactl",
			Args:    []string{"shell", instanceName, "whoami"},
			Output:  username,
		},
		{
			Command: "limactl",
			Args:    []string{"ls", "--format=json"},
			Output:  fmt.Sprintf(`{"name":"%s","status":"Stopped","dir":"%s","vmType":"vz","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":%d,"hostAgentPID":9390,"driverPID":9390}`, instanceName, tmpDir, sshPort),
		},
		{
			Command: "limactl",
			Args:    []string{"shell", "--workdir", "/", instanceName, "ip", "route", "show", "dev", "lima0"},
			Output: fmt.Sprintf(`default via 192.168.64.1 proto dhcp src %s metric 100
192.168.64.0/24 proto kernel scope link src 192.168.64.2
192.168.64.1 proto dhcp scope link src 192.168.64.2 metric 100
`, ip),
		},
		{
			Command: "limactl",
			Args:    []string{"start", instanceName},
			Output:  ``,
		},
	}

	defer command.MockExecCommands(t, commands)()

	if err = manager.CreateInstance(instanceName); err != nil {
		t.Fatal(err)
	}

	if err = manager.StartInstance(instanceName); err != nil {
		t.Fatal(err)
	}

	inventoryContents, err := os.ReadFile(manager.InventoryPath())
	if err != nil {
		t.Fatal(err)
	}

	expectedInventoryContents := fmt.Sprintf(`default ansible_host=127.0.0.1 ansible_port=%d ansible_user=%s ansible_ssh_common_args='-o StrictHostKeyChecking=no'

[development]
default

[web]
default
`, sshPort, username)

	if string(inventoryContents) != expectedInventoryContents {
		t.Errorf("expected inventory file to be %s, got %s", expectedInventoryContents, string(inventoryContents))
	}

	if hostsStorage[instanceName] != ip {
		t.Errorf("expected hosts entry to be %s, got %s", ip, hostsStorage[instanceName])
	}
}

func (h *MockHostsResolver) AddHosts(name string, ip string) error {
	h.Hosts[name] = ip
	return nil
}

func (h *MockHostsResolver) RemoveHosts(name string) error {
	delete(h.Hosts, name)
	return nil
}
