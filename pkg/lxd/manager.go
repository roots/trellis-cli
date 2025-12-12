package lxd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/mitchellh/go-homedir"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

const (
	configDir = "lxd"
)

var (
	ErrConfigPath  = errors.New("could not create config directory")
	defaultSshKeys = []string{"~/.ssh/id_ed25519.pub", "~/.ssh/id_rsa.pub"}
)

type Manager struct {
	ConfigPath    string
	Sites         map[string]*trellis.Site
	HostsResolver vm.HostsResolver
	ui            cli.Ui
	trellis       *trellis.Trellis
}

func NewManager(trellis *trellis.Trellis, ui cli.Ui) (manager *Manager, err error) {
	configPath := filepath.Join(trellis.ConfigPath(), configDir)

	hostNames := trellis.Environments["development"].AllHosts()
	hostsResolver, err := vm.NewHostsResolver(trellis.CliConfig.Vm.HostsResolver, hostNames)

	if err != nil {
		return nil, err
	}

	manager = &Manager{
		ConfigPath:    configPath,
		Sites:         trellis.Environments["development"].WordPressSites,
		HostsResolver: hostsResolver,
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
	name = strings.ReplaceAll(name, ".", "-")
	instances := m.instances()
	instance, ok := instances[name]

	return instance, ok
}

func (m *Manager) CreateInstance(name string) (err error) {
	instance, err := m.newInstance(name)
	if err != nil {
		return err
	}

	if err := instance.CreateConfig(); err != nil {
		return err
	}

	configFile, err := os.Open(instance.ConfigFile)
	if err != nil {
		return err
	}
	defer func() { err = configFile.Close() }()

	version := m.trellis.CliConfig.Vm.Ubuntu

	launchCmd := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("lxc", []string{"launch", "ubuntu:" + version, instance.Name})

	launchCmd.Stdin = configFile
	err = launchCmd.Run()

	if err != nil {
		return err
	}

	return postStart(m, &instance)
}

func (m *Manager) DeleteInstance(name string) error {
	instance, ok := m.GetInstance(name)

	if !ok {
		m.ui.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if instance.Stopped() {
		return command.WithOptions(
			command.WithTermOutput(),
			command.WithLogging(m.ui),
		).Cmd("lxc", []string{"delete", instance.Name}).Run()
	} else {
		return fmt.Errorf("Error: VM is running. Run `trellis vm stop` to stop it.")
	}
}

func (m *Manager) OpenShell(name string, dir string, commandArgs []string) error {
	instance, ok := m.GetInstance(name)

	if !ok {
		m.ui.Info("Instance does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if instance.Stopped() {
		m.ui.Info("Instance is not running. Run `trellis vm start` to start it.")
		return nil
	}

	if dir != "" {
		commandArgs = append([]string{"cd", dir, "&&"}, commandArgs...)
	}

	// TODO: use SSH to IP
	args := []string{"exec", instance.Name, "--", "sh", "-c", strings.Join(commandArgs, " ")}

	return command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("lxc", args).Run()
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

	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("lxc", []string{"start", instance.Name}).Run()

	if err != nil {
		return err
	}

	return postStart(m, &instance)
}

func (m *Manager) StopInstance(name string) error {
	instance, ok := m.GetInstance(name)

	if !ok {
		m.ui.Info("Instance does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if instance.Stopped() {
		m.ui.Info(fmt.Sprintf("%s Instance already stopped", color.GreenString("[✓]")))
		return nil
	}

	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("lxc", []string{"stop", instance.Name}).Run()

	if err != nil {
		return fmt.Errorf("Error stopping Instance\n%v", err)
	}

	if err = m.removeHosts(instance); err != nil {
		return err
	}

	return nil
}

func (m *Manager) hydrateInstance(instance *Instance) error {
	i, _ := m.GetInstance(instance.Name)
	*instance = i

	return nil
}

// TODO: user necessary with privileged containers?
func (m *Manager) initInstance(instance *Instance) {
	user, _ := user.Current()
	instance.Username = user.Username

	info, _ := os.Stat(m.trellis.ConfigPath())

	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		instance.Uid = int(stat.Uid)
		instance.Gid = int(stat.Gid)
	}

	instance.ConfigFile = filepath.Join(m.ConfigPath, instance.Name+".yml")
	instance.InventoryFile = m.InventoryPath()
	instance.Sites = m.Sites
}

func (m *Manager) newInstance(name string) (Instance, error) {
	name = strings.ReplaceAll(name, ".", "-")
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Instance{}, err
	}

	devices := map[string]Device{
		"home": {
			Source: homeDir,
			Dest:   homeDir,
		},
	}

	for name, site := range m.Sites {
		devices[name] = Device{
			Source: site.AbsLocalPath,
			Dest:   "/srv/www/" + name + "/current",
		}
	}

	sshPublicKey := []byte{}

	for _, path := range defaultSshKeys {
		sshPublicKey, err = loadPublicKey(path)

		if err == nil {
			break
		}
	}

	if sshPublicKey == nil {
		return Instance{}, fmt.Errorf("No valid SSH public key found. Attempted paths: %s", strings.Join(defaultSshKeys, ", "))
	}

	instance := Instance{
		Name:         name,
		Devices:      devices,
		SshPublicKey: string(sshPublicKey),
	}

	m.initInstance(&instance)

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

func (m *Manager) instances() (instancesByName map[string]Instance) {
	instances := []Instance{}
	instancesByName = make(map[string]Instance)

	output, _ := command.Cmd("lxc", []string{"list", "--format=json"}).Output()
	_ = json.Unmarshal(output, &instances)

	for _, instance := range instances {
		m.initInstance(&instance)
		instancesByName[instance.Name] = instance
	}

	return instancesByName
}

func (m *Manager) removeHosts(instance Instance) error {
	return m.HostsResolver.RemoveHosts(instance.Name)
}

func (m *Manager) retryHydrateInstance(instance *Instance, ctx context.Context) error {
	interval := 1 * time.Second

	for {
		err := m.hydrateInstance(instance)
		if err != nil {
			return err
		}

		_, err = instance.IP()
		if err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout hydrating VM: %v", err)
		case <-time.After(interval):
		}
	}
}

func postStart(manager *Manager, instance *Instance) error {
	// Hydrate instance with data from lxc that is only available after starting (mainly the IP)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := manager.retryHydrateInstance(instance, ctx)
	if err != nil {
		return err
	}

	if err = manager.addHosts(*instance); err != nil {
		return err
	}

	return nil
}

func loadPublicKey(path string) ([]byte, error) {
	path, err := homedir.Expand(path)
	if err != nil {
		return nil, err
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return contents, nil
}
