package lima

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mcuadros/go-version"
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
	trellis       *trellis.Trellis
}

func NewManager(trellis *trellis.Trellis) (manager *Manager, err error) {
	macOSVersion, err := getMacOSVersion()
	if err != nil {
		return nil, fmt.Errorf("%w\n%v", UnsupportedOSError, err)
	}

	if version.Compare(macOSVersion, RequiredMacOSVersion, "<") {
		return nil, fmt.Errorf("%w", UnsupportedOSError)
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
	}

	if err = manager.createConfigPath(); err != nil {
		return nil, fmt.Errorf("%w: %v", ConfigPathError, err)
	}

	return manager, nil
}

func (m *Manager) NewInstance(name string) Instance {
	name = convertToInstanceName(name)
	images := []Image{}

	for _, image := range m.trellis.CliConfig.Vm.Images {
		images = append(images, Image{
			Location: image.Location,
			Arch:     image.Arch,
		})
	}

	instance := Instance{
		Name:       name,
		ConfigPath: m.ConfigPath,
		ConfigFile: filepath.Join(m.ConfigPath, name+".yml"),
		Images:     images,
		Sites:      m.Sites,
	}

	return instance
}

func (m *Manager) GetInstance(name string) (Instance, bool) {
	instances := m.Instances()
	instance, ok := instances[convertToInstanceName(name)]

	return instance, ok
}

func (m *Manager) Instances() (instances map[string]Instance) {
	instances = make(map[string]Instance)
	output, err := command.Cmd("limactl", []string{"list", "--json"}).Output()

	for _, line := range strings.Split(string(output), "\n") {
		var instance Instance
		if err = json.Unmarshal([]byte(line), &instance); err == nil {
			instances[instance.Name] = instance
		}
	}

	return instances
}

func (m *Manager) createConfigPath() error {
	return os.MkdirAll(m.ConfigPath, 0755)
}

func convertToInstanceName(value string) string {
	return strings.ReplaceAll(value, ".", "-")
}

func getMacOSVersion() (string, error) {
	if runtime.GOOS != "darwin" {
		return "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	cmd := command.Cmd("sw_vers", []string{"-productVersion"})
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to execute %v: %w", cmd.Args, err)
	}
	verTrimmed := strings.TrimSpace(string(b))
	version := version.Normalize(verTrimmed)
	return version, nil
}
