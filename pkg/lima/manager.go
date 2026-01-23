package lima

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
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
	RequiredLinuxVersion = "20.04"
)

var (
	ErrConfigPath    = errors.New("could not create config directory")
	ErrUnsupportedOS = errors.New("unsupported OS or macOS version. The macOS Virtualization Framework requires macOS 13.0 (Ventura) or later.")
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
		if err := ensureRequirements(); err != nil {
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
		// 1. Ensure tap0 exists (unchanged)
		if _, err := exec.Command("ip", "link", "show", "tap0").Output(); err != nil {
			m.ui.Info(color.YellowString("🔧 tap0 missing. Creating it (requires sudo)..."))
			// Added 'set tap0 up' to ensure the link is active on host
			cmd := exec.Command("sudo", "sh", "-c", "ip tuntap add dev tap0 mode tap user $(whoami) && ip addr add 192.168.56.1/24 dev tap0 && ip link set tap0 up")
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return fmt.Errorf("Failed to create tap0: %v", err)
			}
		}

		m.ui.Info(color.YellowString("🛠️  Configuring QEMU Network Wrapper..."))

		// 2. Create a dedicated directory for the wrapper
		wrapperDir := filepath.Join(os.TempDir(), "lima-qemu-wrapper")
		if err := os.MkdirAll(wrapperDir, 0755); err != nil {
			return fmt.Errorf("failed to create wrapper dir: %v", err)
		}

		// 3. Name the script exactly 'qemu-system-x86_64' so Lima picks it up
		wrapperPath := filepath.Join(wrapperDir, "qemu-system-x86_64")
		
		wrapperScript := `#!/bin/bash
# Pass-through for help/version commands to prevent crashing during capability checks
if [[ "$@" == *"-netdev help"* ]] || \
[[ "$@" == *"-version"* ]] || \
[[ "$@" == *"-accel help"* ]] || \
[[ "$@" == *"-machine help"* ]] || \
[[ "$@" == *"-cpu help"* ]]; then
	exec /usr/bin/qemu-system-x86_64 "$@"
fi

# Inject the TAP interface args BEFORE other args ($@) to ensure precedence
exec /usr/bin/qemu-system-x86_64 \
-netdev tap,id=mynet0,ifname=tap0,script=no,downscript=no \
-device virtio-net-pci,netdev=mynet0,mac=52:54:00:12:34:56 \
"$@"
`
		if err := os.WriteFile(wrapperPath, []byte(wrapperScript), 0755); err != nil {
			m.ui.Warn(fmt.Sprintf("Failed to create QEMU wrapper: %v", err))
		}

		// 4. CRITICAL: Prepend the wrapper directory to PATH
		// This forces Lima to find your script before the real QEMU
		currentPath := os.Getenv("PATH")
		newPath := wrapperDir + string(os.PathListSeparator) + currentPath
		os.Setenv("PATH", newPath)
	}




	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", []string{"start", instance.Name}).Run()

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

func getLinuxVersion() (string, error) {
    // /etc/os-release is the standard on almost all modern Linux distros
    f, err := os.Open("/etc/os-release")
    if err != nil {
        return "", err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := scanner.Text()
        // Look for the line starting with VERSION_ID=
        if strings.HasPrefix(line, "VERSION_ID=") {
            // Remove the prefix
            val := strings.TrimPrefix(line, "VERSION_ID=")
            // Remove double or single quotes if present (e.g. "22.04" -> 22.04)
            val = strings.Trim(val, `"'`)
            
            // Normalize using your existing version package
            verNormalized := version.Normalize(val)
            return verNormalized, nil
        }
    }

    if err := scanner.Err(); err != nil {
        return "", err
    }

    return "", fmt.Errorf("VERSION_ID not found in /etc/os-release")
}

func ensureRequirements() error {
	var currentVersion string
	var requiredVersion string
	var err error

	switch runtime.GOOS {
		case "darwin":
			currentVersion, err = getMacOSVersion()
			requiredVersion = RequiredMacOSVersion
		case "linux":
			currentVersion, err = getLinuxVersion()
			requiredVersion = RequiredLinuxVersion
		default:
			return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
    if err != nil {
        return fmt.Errorf("failed to detect OS version: %w", err)
    }

	// Now compare the current version against the specific requirement for that OS
	if version.Compare(currentVersion, requiredVersion, "<") {
		return fmt.Errorf("OS version %s is too old; %s or newer is required", currentVersion, requiredVersion)
	}

	if err = Installed(); err != nil {
		return fmt.Errorf("%s\nInstall or upgrade Lima to continue:\n\n  brew install lima\n\nSee https://github.com/lima-vm/lima#getting-started for manual installation options.", err.Error())
	}

	return nil
}

func imagesFromVersion(version string) []Image {
	return UbuntuImages[version]
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
