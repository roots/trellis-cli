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
	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

const (
	configDir            = "lima"
	RequiredMacOSVersion = "13.0.0"
)

var (
	DataDirError       = errors.New("could not create data directory")
	ConfigPathError    = errors.New("could not create config directory")
	UnsupportedOSError = errors.New("Unsupported OS or macOS version. The macOS Virtualization Framework requires macOS 13.0 (Ventura) or later.")
)

type Manager struct {
	DataDir       string
	ConfigPath    string
	Sites         map[string]*trellis.Site
	HostsResolver HostsResolver
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

	manager = &Manager{
		DataDir:       app_paths.DataDir(),
		ConfigPath:    limaConfigPath,
		Sites:         trellis.Environments["development"].WordPressSites,
		HostsResolver: NewHostsResolver(trellis.CliConfig.VmHostsResolver, hostNames),
	}

	if err = manager.createDataDir(); err != nil {
		return nil, fmt.Errorf("%w: %v", DataDirError, err)
	}
	if err = manager.createConfigPath(); err != nil {
		return nil, fmt.Errorf("%w: %v", ConfigPathError, err)
	}

	manager.setPath()

	return manager, nil
}

func (m *Manager) NewInstance(name string) Instance {
	name = convertToInstanceName(name)

	instance := Instance{
		Name:       name,
		ConfigPath: m.ConfigPath,
		ConfigFile: filepath.Join(m.ConfigPath, name+".yml"),
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

func (m *Manager) createDataDir() error {
	return os.MkdirAll(m.DataDir, 0755)
}

func (m *Manager) createConfigPath() error {
	return os.MkdirAll(m.ConfigPath, 0755)
}

func (m *Manager) setPath() {
	osPath := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:%s", m.DataDir, osPath))
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
