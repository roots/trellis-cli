package lima

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

type MockHostsResolver struct {
	Hosts map[string]string
}

type MockPortFinder struct{}

func (p *MockPortFinder) Resolve() (int, error) {
	return 60720, nil
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
			Command: "limactl",
			Args:    []string{"-v"},
			Output:  `limactl version 0.15.0`,
		},
	}
	if runtime.GOOS == "darwin" {
		commands = append([]command.MockCommand{
			{
				Command: "sw_vers",
				Args:    []string{"-productVersion"},
				Output:  `13.0.1`,
			},
		}, commands...)
	}
	defer command.MockExecCommands(t, commands)()

	_, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewManagerUnsupportedOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS-specific test")
	}

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

	expected := "unsupported OS. Lima VM requires macOS 13.0+ or Linux."

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

	trellis.CliConfig.Vm.Ubuntu = "22.04"

	t.Setenv("TRELLIS_BYPASS_LIMA_REQUIREMENTS", "1")

	manager, err := NewManager(trellis, cli.NewMockUi())
	if err != nil {
		t.Fatal(err)
	}

	manager.PortFinder = &MockPortFinder{}

	instance, err := manager.newInstance("test")

	if err != nil {
		t.Fatal(err)
	}

	if instance.Name != "test" {
		t.Errorf("expected instance name to be %q, got %q", "test", instance.Name)
	}

	if len(instance.Config.Images) != 2 {
		t.Errorf("expected instance config to have 2 images, got %d", len(instance.Config.Images))
	}

	if instance.Config.Images[0].Alias != "jammy" {
		t.Errorf("expected instance config to have jammy image, got %q", instance.Config.Images[0].Alias)
	}

	if len(instance.Config.PortForwards) != 1 {
		t.Errorf("expected instance config to have 1 port forwards, got %d", len(instance.Config.PortForwards))
	}

	if instance.Config.PortForwards[0].GuestPort != 80 || instance.Config.PortForwards[0].HostPort != 60720 {
		t.Errorf("expected instance config to have port forward guest 80 to host 60720, got guest %d to host %d", instance.Config.PortForwards[0].GuestPort, instance.Config.PortForwards[0].HostPort)
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

	vmType := "vz"
	if runtime.GOOS == "linux" {
		vmType = "qemu"
	}
	instancesJson := fmt.Sprintf(`{"name":"%s","status":"Running","dir":"/foo/test","vmType":"%s","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":60720,"hostAgentPID":9390,"driverPID":9390}
{"name":"test2","status":"Running","dir":"/foo/test","vmType":"%s","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":60720,"hostAgentPID":9390,"driverPID":9390}`, instanceName, vmType, vmType)

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
	manager.PortFinder = &MockPortFinder{}

	instanceName := "test"
	sshPort := 60720
	username := "user1"
	vmType := "vz"
	if runtime.GOOS == "linux" {
		vmType = "qemu"
	}

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
			Output:  fmt.Sprintf(`{"name":"%s","status":"Stopped","dir":"/foo/test","vmType":"%s","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":%d,"hostAgentPID":9390,"driverPID":9390}`, instanceName, vmType, sshPort),
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
	if runtime.GOOS == "linux" {
		_, _ = os.OpenFile(filepath.Join(tmpDir, "qemu-system-aarch64"), os.O_CREATE, 0755)
		path := os.Getenv("PATH")
		t.Setenv("PATH", tmpDir+string(os.PathListSeparator)+path)
	}

	hostsStorage := make(map[string]string)
	manager.HostsResolver = &MockHostsResolver{Hosts: hostsStorage}
	manager.PortFinder = &MockPortFinder{}

	instanceName := "test"
	sshPort := 60720
	username := "user1"
	ip := "192.168.64.2"
	vmType := "vz"
	expectedHostIP := ip
	if runtime.GOOS == "linux" {
		vmType = "qemu"
		ip = "192.168.56.5"
		expectedHostIP = ip
	}

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
			Output:  fmt.Sprintf(`{"name":"%s","status":"Stopped","dir":"%s","vmType":"%s","arch":"aarch64","cpuType":"","cpus":4,"memory":4294967296,"disk":107374182400,"network":[{"vzNAT":true,"macAddress":"52:55:55:6f:d9:e3","interface":"lima0"}],"sshLocalPort":%d,"hostAgentPID":9390,"driverPID":9390}`, instanceName, tmpDir, vmType, sshPort),
		},
		{
			Command: "limactl",
			Args:    []string{"start", instanceName},
			Output:  ``,
		},
	}
	if runtime.GOOS == "linux" {
		currentUser, userErr := user.Current()
		if userErr != nil {
			t.Fatal(userErr)
		}
		commands = append(commands,
			command.MockCommand{
				Command:  "ip",
				Args:     []string{"tuntap", "show"},
				Output:   "tap0: tap persist user 0\n",
				ExitCode: 0,
			},
			command.MockCommand{
				Command:  "ip",
				Args:     []string{"-4", "addr", "show", "dev", linuxTapDevice},
				Output:   "",
				ExitCode: 1,
			},
			command.MockCommand{
				Command: "sudo",
				Args: []string{"sh", "-c", fmt.Sprintf(`if ip tuntap show | grep -q '^%s:'; then
  ip tuntap del dev %s mode tap 2>/dev/null || true
fi
ip tuntap add dev %s mode tap user %s
ip addr replace %s dev %s
ip link set %s up`,
					linuxTapDevice, linuxTapDevice, linuxTapDevice, currentUser.Uid, linuxTapHostCIDR, linuxTapDevice, linuxTapDevice)},
				Output: ``,
			},
		)
		commands = append(commands, command.MockCommand{
			Command: "limactl",
			Args:    []string{"shell", "--workdir", "/", instanceName, "ip", "route", "show"},
			Output: fmt.Sprintf(`default via 192.168.5.1 dev lima0 proto dhcp src 192.168.5.15 metric 100
192.168.56.0/24 dev enp0s8 proto kernel scope link src %s`, ip),
		})
	} else {
		commands = append(commands, command.MockCommand{
			Command: "limactl",
			Args:    []string{"shell", "--workdir", "/", instanceName, "ip", "route", "show", "dev", "lima0"},
			Output: fmt.Sprintf(`default via 192.168.64.1 proto dhcp src %s metric 100
192.168.64.0/24 proto kernel scope link src 192.168.64.2
192.168.64.1 proto dhcp scope link src 192.168.64.2 metric 100
`, ip),
		})
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

	if hostsStorage[instanceName] != expectedHostIP {
		t.Errorf("expected hosts entry to be %s, got %s", expectedHostIP, hostsStorage[instanceName])
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
