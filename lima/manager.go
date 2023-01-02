package lima

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcuadros/go-version"
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

const (
	configDir            = "lima"
	RequiredMacOSVersion = "13.0.0"
)

var (
	ConfigPathError    = errors.New("could not create config directory")
	UnsupportedOSError = errors.New("Unsupported OS or macOS version. The macOS Virtualization Framework requires macOS 13.0 (Ventura) or later.")
)

type Manager struct {
	ConfigPath    string
	Sites         map[string]*trellis.Site
	HostsResolver HostsResolver
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
	hostsResolver, err := NewHostsResolver(trellis.CliConfig.Vm.HostsResolver, hostNames)

	if err != nil {
		return nil, err
	}

	manager = &Manager{
		ConfigPath:    limaConfigPath,
		Sites:         trellis.Environments["development"].WordPressSites,
		HostsResolver: hostsResolver,
		trellis:       trellis,
		ui:            ui,
	}

	if err = manager.createConfigPath(); err != nil {
		return nil, fmt.Errorf("%w: %v", ConfigPathError, err)
	}

	return manager, nil
}

func (m *Manager) GetInstance(name string) (Instance, bool) {
	instances := m.Instances()
	instance, ok := instances[name]

	return instance, ok
}

func (m *Manager) Instances() (instancesByName map[string]Instance) {
	instances := []Instance{}
	instancesByName = make(map[string]Instance)

	output, _ := command.Cmd("limactl", []string{"inspect"}).Output()
	json.Unmarshal(output, &instances)

	for _, instance := range instances {
		m.initInstance(&instance)
		instancesByName[instance.Name] = instance
	}

	return instancesByName
}

func (m *Manager) CreateInstance(name string) error {
	httpForwardPort, err := findFreeTCPLocalPort()
	if err != nil {
		return fmt.Errorf("Could not find a local free port for HTTP forwarding: %v", err)
	}

	instance := m.newInstance(name)

	instance.Config.PortForwards = []PortForward{
		{GuestPort: 80, HostPort: httpForwardPort},
	}

	if err := instance.CreateConfig(); err != nil {
		return err
	}

	return m.StartInstance(instance)
}

func (m *Manager) DeleteInstance(instance Instance) error {
	return command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", []string{"delete", instance.Name}).Run()
}

// TODO: set working dir to site path?
func (m *Manager) OpenShell(instance Instance, commandArgs []string) error {
	args := []string{"shell", instance.Name}
	args = append(args, commandArgs...)

	return command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", args).Run()
}

func (m *Manager) StartInstance(instance Instance) error {
	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", []string{"start", "--tty=false", "--name=" + instance.Name, instance.ConfigFile}).Run()

	if err != nil {
		return err
	}

	user, err := instance.getUsername()
	if err != nil {
		return fmt.Errorf("Could not get username: %v", err)
	}

	instance.Username = string(user)

	// Hydrate instance with data from limactl that is only available after starting (mainly the forwarded SSH local port)
	err = m.hydrateInstance(&instance)
	if err != nil {
		return err
	}

	if err = m.addHosts(instance); err != nil {
		return err
	}

	return nil
}

func (m *Manager) StopInstance(instance Instance) error {
	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("limactl", []string{"stop", instance.Name}).Run()

	if err != nil {
		return err
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
	instance.ConfigPath = m.ConfigPath
	instance.ConfigFile = filepath.Join(m.ConfigPath, instance.Name+".yml")
	instance.Sites = m.Sites
}

func (m *Manager) newInstance(name string) Instance {
	instance := Instance{Name: name}
	m.initInstance(&instance)

	images := []Image{}

	for _, image := range m.trellis.CliConfig.Vm.Images {
		images = append(images, Image{
			Location: image.Location,
			Arch:     image.Arch,
		})
	}

	config := Config{Images: images}
	instance.Config = config
	return instance
}

func (m *Manager) createConfigPath() error {
	return os.MkdirAll(m.ConfigPath, 0755)
}

func (m *Manager) addHosts(instance Instance) error {
	if err := instance.CreateInventoryFile(); err != nil {
		return err
	}

	if err := m.HostsResolver.AddHosts(instance.Name, &instance); err != nil {
		return err
	}

	return nil
}

func (m *Manager) removeHosts(instance Instance) error {
	return m.HostsResolver.RemoveHosts(instance.Name, &instance)
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

func ensureRequirements() error {
	macOSVersion, err := getMacOSVersion()
	if err != nil {
		return UnsupportedOSError
	}

	if version.Compare(macOSVersion, RequiredMacOSVersion, "<") {
		return fmt.Errorf("%w", UnsupportedOSError)
	}

	if err = Installed(); err != nil {
		return fmt.Errorf(err.Error() + `
Install or upgrade Lima to continue:

  brew install lima

See https://github.com/lima-vm/lima#getting-started for manual installation options.`)
	}

	return nil
}

func findFreeTCPLocalPort() (int, error) {
	lAddr0, err := net.ResolveTCPAddr("tcp4", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp4", lAddr0)
	if err != nil {
		return 0, err
	}
	defer l.Close()
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
