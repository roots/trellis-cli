package lima

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/roots/trellis-cli/app_paths"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

const configDir = "lima"

var (
	DataDirError    = errors.New("could not create data directory")
	ConfigPathError = errors.New("could not create config directory")
)

type Manager struct {
	DataDir    string
	ConfigPath string
	Sites      map[string]*trellis.Site
}

func NewManager(configPath string, sites map[string]*trellis.Site) (manager *Manager, err error) {
	limaConfigPath := filepath.Join(configPath, configDir)

	manager = &Manager{
		DataDir:    app_paths.DataDir(),
		ConfigPath: limaConfigPath,
		Sites:      sites,
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
