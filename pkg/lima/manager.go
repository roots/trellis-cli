package lima

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
)

var (
	ConfigPathError    = errors.New("could not create config directory")
	UnsupportedOSError = errors.New("Unsupported OS or macOS version. The macOS Virtualization Framework requires macOS 13.0 (Ventura) or later.")
)

type Manager struct {
	ConfigPath    string
	Sites         map[string]*trellis.Site
	HostsResolver vm.HostsResolver
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
		trellis:       trellis,
		ui:            ui,
	}

	if err = manager.createConfigPath(); err != nil {
		return nil, fmt.Errorf("%w: %v", ConfigPathError, err)
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
	instance := m.newInstance(name)

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

func (m *Manager) StartInstance(name string) error {
	instance, ok := m.GetInstance(name)

	if !ok {
		return vm.VmNotFoundErr
	}

	if instance.Running() {
		m.ui.Info(fmt.Sprintf("%s VM already running", color.GreenString("[✓]")))
		return nil
	}

	if err := instance.UpdateConfig(); err != nil {
		return err
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

func (m *Manager) newInstance(name string) Instance {
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
		json.Unmarshal([]byte(line), instance)
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

func imagesFromVersion(version string) []Image {
	return UbuntuImages[version]
}
