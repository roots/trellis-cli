package lima

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/mcuadros/go-version"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

const (
	configDir            = "lima"
	RequiredMacOSVersion = "13.0.0"
	// Linux TAP networking constants. These are hardcoded to a single set of
	// values, which means only one Lima VM can run at a time on Linux.
	linuxTapDevice     = "tap0"
	linuxTapHostCIDR   = "192.168.56.1/24"
	linuxTapMACAddress = "52:54:00:12:34:56"
)

var (
	ErrConfigPath    = errors.New("could not create config directory")
	ErrUnsupportedOS = errors.New("unsupported OS. Lima VM requires macOS 13.0+ or Linux.")
)

type PortFinder interface {
	Resolve() (int, error)
}

type TCPPortFinder struct{}

type Manager struct {
	ConfigPath    string
	Sites         map[string]*trellis.Site
	HostsResolver vm.HostsResolver
	PortFinder    PortFinder
	ui            cli.Ui
	trellis       *trellis.Trellis
}

func NewManager(trellis *trellis.Trellis, ui cli.Ui) (manager *Manager, err error) {
	if os.Getenv("TRELLIS_BYPASS_LIMA_REQUIREMENTS") != "1" {
		if err := ensureRequirements(ui); err != nil {
			return nil, err
		}
	}

	limaConfigPath := filepath.Join(trellis.ConfigPath(), configDir)

	hostNames := trellis.Environments["development"].AllHosts()
	hostsResolver, err := vm.NewHostsResolver(trellis.CliConfig.Vm.HostsResolver, hostNames)

	if err != nil {
		return nil, err
	}

	manager = &Manager{
		ConfigPath:    limaConfigPath,
		Sites:         trellis.Environments["development"].WordPressSites,
		HostsResolver: hostsResolver,
		PortFinder:    &TCPPortFinder{},
		trellis:       trellis,
		ui:            ui,
	}

	if err = manager.createConfigPath(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigPath, err)
	}

	return manager, nil
}

func (m *Manager) InventoryPath() string {
	return filepath.Join(m.ConfigPath, "inventory")
}

func (m *Manager) GetInstance(name string) (Instance, bool) {
	instances := m.instances()
	instance, ok := instances[name]

	return instance, ok
}

func (m *Manager) CreateInstance(name string) error {
	instance, err := m.newInstance(name)
	if err != nil {
		return err
	}

	cmd := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", []string{"create", "--tty=false", "--name=" + instance.Name, "-"})

	configContents, err := instance.GenerateConfig()
	if err != nil {
		return err
	}

	cmd.Stdin = configContents
	return cmd.Run()
}

func (m *Manager) DeleteInstance(name string) error {
	instance, ok := m.GetInstance(name)

	if !ok {
		m.ui.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if instance.Stopped() {
		err := command.WithOptions(
			command.WithTermOutput(),
			command.WithLogging(m.ui),
		).Cmd("limactl", []string{"delete", instance.Name}).Run()

		if err != nil {
			return err
		}

		return nil
	} else {
		return fmt.Errorf("Error: VM is running. Run `trellis vm stop` to stop it.")
	}
}

func (m *Manager) OpenShell(name string, dir string, commandArgs []string) error {
	instance, ok := m.GetInstance(name)

	if !ok {
		m.ui.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if instance.Stopped() {
		m.ui.Info("VM is not running. Run `trellis vm start` to start it.")
		return nil
	}

	args := []string{"shell", "--workdir", dir, instance.Name}
	args = append(args, commandArgs...)

	return command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", args).Run()
}

func (m *Manager) RunCommand(args []string, dir string) error {
	instanceName, err := m.trellis.GetVmInstanceName()
	if err != nil {
		return err
	}

	instance, ok := m.GetInstance(instanceName)
	if !ok {
		return fmt.Errorf("VM does not exist. Run `trellis vm start` to create it.")
	}
	if instance.Stopped() {
		return fmt.Errorf("VM is not running. Run `trellis vm start` to start it.")
	}

	shellArgs := []string{"shell"}
	if dir != "" {
		shellArgs = append(shellArgs, "--workdir", dir)
	}
	shellArgs = append(shellArgs, instance.Name)
	shellArgs = append(shellArgs, args...)

	return command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", shellArgs).Run()
}

func (m *Manager) RunCommandPipe(args []string, dir string) (*exec.Cmd, error) {
	instanceName, err := m.trellis.GetVmInstanceName()
	if err != nil {
		return nil, err
	}

	instance, ok := m.GetInstance(instanceName)
	if !ok {
		return nil, fmt.Errorf("VM does not exist. Run `trellis vm start` to create it.")
	}
	if instance.Stopped() {
		return nil, fmt.Errorf("VM is not running. Run `trellis vm start` to start it.")
	}

	shellArgs := []string{"shell"}
	if dir != "" {
		shellArgs = append(shellArgs, "--workdir", dir)
	}
	shellArgs = append(shellArgs, instance.Name)
	shellArgs = append(shellArgs, args...)

	return command.Cmd("limactl", shellArgs), nil
}

func (m *Manager) StartInstance(name string) error {
	instance, ok := m.GetInstance(name)

	if !ok {
		return vm.ErrVmNotFound
	}

	if instance.Running() {
		m.ui.Info(fmt.Sprintf("%s VM already running", color.GreenString("[✓]")))
		return nil
	}

	if err := instance.UpdateConfig(); err != nil {
		return err
	}

	if runtime.GOOS == "linux" {
		if err := m.ensureLinuxTapDevice(); err != nil {
			return err
		}
	}

	cmd := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", []string{"start", instance.Name})

	if runtime.GOOS == "linux" {
		wrapperDir, err := m.ensureQEMUWrapper()
		if err != nil {
			return err
		}

		cmd.Env = append(os.Environ(), fmt.Sprintf("PATH=%s%s%s", wrapperDir, string(os.PathListSeparator), os.Getenv("PATH")))
	}

	err := cmd.Run()

	if err != nil {
		return err
	}

	user, err := instance.getUsername()
	if err != nil {
		return fmt.Errorf("Could not get username: %v", err)
	}

	instance.Username = string(user)

	// Hydrate instance with data from limactl that is only available after
	// starting (mainly the forwarded local ports)
	err = m.hydrateInstance(&instance)
	if err != nil {
		return err
	}

	if err = m.addHosts(instance); err != nil {
		return err
	}

	return nil
}

func (m *Manager) StopInstance(name string) error {
	instance, ok := m.GetInstance(name)

	if !ok {
		m.ui.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if instance.Stopped() {
		m.ui.Info(fmt.Sprintf("%s VM already stopped", color.GreenString("[✓]")))
		return nil
	}

	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", []string{"stop", instance.Name}).Run()

	if err != nil {
		return fmt.Errorf("Error stopping VM\n%v", err)
	}

	if err = m.removeHosts(instance); err != nil {
		return err
	}

	return nil
}

func (m *Manager) hydrateInstance(instance *Instance) error {
	i, _ := m.GetInstance(instance.Name)
	tmpJson, err := json.Marshal(i)
	if err != nil {
		return fmt.Errorf("Could not marshal instance: %v\nThis is a trellis-cli bug.", err)
	}
	if err = json.Unmarshal(tmpJson, instance); err != nil {
		return fmt.Errorf("Could not unmarshal instance: %v\nThis is a trellis-cli bug.", err)
	}

	return nil
}

func (m *Manager) initInstance(instance *Instance) {
	instance.InventoryFile = m.InventoryPath()
	instance.Sites = m.Sites
}

func (m *Manager) newInstance(name string) (Instance, error) {
	instance := Instance{Name: name}
	if runtime.GOOS == "linux" {
		instance.VMType = "qemu"
	} else {
		instance.VMType = "vz"
	}
	m.initInstance(&instance)

	images := []Image{}

	if len(m.trellis.CliConfig.Vm.Images) > 0 {
		for _, image := range m.trellis.CliConfig.Vm.Images {
			images = append(images, Image{
				Location: image.Location,
				Arch:     image.Arch,
			})
		}
	} else {
		images = imagesFromVersion(m.trellis.CliConfig.Vm.Ubuntu)
	}

	portForwards := []PortForward{}

	if m.trellis.CliConfig.Vm.ForwardHttpPort {
		httpForwardPort, err := m.PortFinder.Resolve()
		if err != nil {
			return Instance{}, fmt.Errorf("Could not find a local free port for HTTP forwarding: %v", err)
		}

		portForwards = append(portForwards, PortForward{
			GuestPort: 80,
			HostPort:  httpForwardPort,
		},
		)
	}

	config := Config{Images: images, PortForwards: portForwards}
	instance.Config = config
	return instance, nil
}

func (m *Manager) createConfigPath() error {
	return os.MkdirAll(m.ConfigPath, 0755)
}

func (m *Manager) addHosts(instance Instance) error {
	if err := instance.CreateInventoryFile(); err != nil {
		return err
	}

	ip, err := instance.IP()
	if err != nil {
		return err
	}

	if err := m.HostsResolver.AddHosts(instance.Name, ip); err != nil {
		return err
	}

	return nil
}

func (m *Manager) instances() (instances map[string]Instance) {
	instances = make(map[string]Instance)

	// Returns line delimited JSON
	output, _ := command.Cmd("limactl", []string{"ls", "--format=json"}).Output()

	for _, line := range bytes.Split(output, []byte("\n")) {
		instance := &Instance{}
		if err := json.Unmarshal([]byte(line), instance); err != nil {
			continue
		}
		m.initInstance(instance)
		instances[instance.Name] = *instance
	}

	return instances
}

func (m *Manager) removeHosts(instance Instance) error {
	return m.HostsResolver.RemoveHosts(instance.Name)
}

func getMacOSVersion() (string, error) {
	cmd := command.Cmd("sw_vers", []string{"-productVersion"})
	b, err := cmd.Output()
	if err != nil {
		return "", err
	}

	verTrimmed := strings.TrimSpace(string(b))
	version := version.Normalize(verTrimmed)
	return version, nil
}

func ensureRequirements(ui cli.Ui) error {
	switch runtime.GOOS {
	case "darwin":
		macOSVersion, err := getMacOSVersion()
		if err != nil {
			return ErrUnsupportedOS
		}

		if version.Compare(macOSVersion, RequiredMacOSVersion, "<") {
			return fmt.Errorf("%w", ErrUnsupportedOS)
		}
	case "linux":
		checkKVM(ui)
	default:
		return ErrUnsupportedOS
	}

	if err := Installed(); err != nil {
		if runtime.GOOS == "linux" && errors.Is(err, ErrUnparseableVersion) && HasHashVersionOutput(err.Error()) {
			ui.Warn(fmt.Sprintf("Warning: %s\nProceeding because this Linux Lima package reports a git-hash version string.", err.Error()))
			return nil
		}

		return fmt.Errorf("%s\nInstall or upgrade Lima to continue.\nSee https://lima-vm.io/docs/installation/", err.Error())
	}

	return nil
}

func imagesFromVersion(version string) []Image {
	return UbuntuImages[version]
}

func checkKVM(ui cli.Ui) {
	f, err := os.OpenFile("/dev/kvm", os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			ui.Warn("Warning: KVM is not available (/dev/kvm not found). QEMU will run without hardware acceleration and will be very slow.\nEnsure your CPU supports virtualization and it is enabled in BIOS.")
		} else {
			ui.Warn(fmt.Sprintf("Warning: Cannot access /dev/kvm: %v\nQEMU will run without hardware acceleration and will be very slow.\nYou may need to add your user to the 'kvm' group:\n\n  sudo usermod -aG kvm $USER\n\nThen log out and back in.", err))
		}
		return
	}

	_ = f.Close()
}

func (m *Manager) ensureLinuxTapDevice() error {
	currentUser, err := user.Current()
	if err != nil {
		return fmt.Errorf("Could not determine current user for Linux TAP setup: %v", err)
	}

	tuntapOutput, _ := command.Cmd("ip", []string{"tuntap", "show"}).CombinedOutput()
	linkOutput, linkErr := command.Cmd("ip", []string{"-4", "addr", "show", "dev", linuxTapDevice}).CombinedOutput()
	expectedOwner := fmt.Sprintf("user %s", currentUser.Uid)

	if strings.Contains(string(tuntapOutput), linuxTapDevice+":") &&
		strings.Contains(string(tuntapOutput), expectedOwner) &&
		linkErr == nil &&
		strings.Contains(string(linkOutput), "192.168.56.1/24") {
		return nil
	}

	m.ui.Info("Preparing Linux TAP interface for host-reachable VM networking...")

	cmd := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("sudo", []string{
		"sh",
		"-c",
		fmt.Sprintf(
			`if ip tuntap show | grep -q '^%s:'; then
  ip tuntap del dev %s mode tap 2>/dev/null || true
fi
ip tuntap add dev %s mode tap user %s
ip addr replace %s dev %s
ip link set %s up`,
			linuxTapDevice,
			linuxTapDevice,
			linuxTapDevice,
			currentUser.Uid,
			linuxTapHostCIDR,
			linuxTapDevice,
			linuxTapDevice,
		),
	})

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("Could not configure Linux TAP interface %q: %v", linuxTapDevice, err)
	}

	return nil
}

func (m *Manager) ensureQEMUWrapper() (string, error) {
	wrapperDir := filepath.Join(m.ConfigPath, "bin")
	if err := os.MkdirAll(wrapperDir, 0755); err != nil {
		return "", fmt.Errorf("Could not create QEMU wrapper directory: %v", err)
	}

	found := false
	for _, binaryName := range []string{"qemu-system-x86_64", "qemu-system-aarch64"} {
		realPath, err := exec.LookPath(binaryName)
		if err != nil {
			continue
		}

		found = true
		wrapperPath := filepath.Join(wrapperDir, binaryName)
		wrapperScript := fmt.Sprintf(`#!/bin/bash
set -e

# Only inject TAP networking args for real VM launches.
# Lima probes QEMU with help/version flags that must pass through unchanged.
has_netdev=false
for arg in "$@"; do
  case "$arg" in
    -netdev) has_netdev=true; break ;;
  esac
done

if [ "$has_netdev" = false ]; then
  exec %q "$@"
fi

exec %q \
  -netdev tap,id=trellis-tap0,ifname=%s,script=no,downscript=no \
  -device virtio-net-pci,netdev=trellis-tap0,mac=%s \
  "$@"
`, realPath, realPath, linuxTapDevice, linuxTapMACAddress)

		if err := os.WriteFile(wrapperPath, []byte(wrapperScript), 0755); err != nil {
			return "", fmt.Errorf("Could not write QEMU wrapper %q: %v", wrapperPath, err)
		}
	}

	if !found {
		return "", fmt.Errorf("Could not find a QEMU system binary in PATH. Install QEMU so Lima can launch Linux VMs.")
	}

	return wrapperDir, nil
}

func (p *TCPPortFinder) Resolve() (int, error) {
	lAddr0, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp4", lAddr0)
	if err != nil {
		return 0, err
	}

	defer func() { _ = l.Close() }()
	lAddr := l.Addr()

	lTCPAddr, ok := lAddr.(*net.TCPAddr)
	if !ok {
		return 0, fmt.Errorf("expected *net.TCPAddr, got %v", lAddr)
	}

	port := lTCPAddr.Port

	if port <= 0 {
		return 0, fmt.Errorf("unexpected port %d", port)
	}

	return port, nil
}
