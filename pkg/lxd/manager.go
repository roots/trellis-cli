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
	"time"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/mitchellh/go-homedir"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

const (
	configDir = "lxd"
)

var (
	ConfigPathError    = errors.New("could not create config directory")
	UnsupportedOSError = errors.New("Unsupported OS or macOS version. The macOS Virtualization Framework requires macOS 13.0 (Ventura) or later.")
	defaultSshKeys     = []string{"~/.ssh/id_ed25519.pub", "~/.ssh/id_rsa.pub"}
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
		return nil, fmt.Errorf("%w: %v", ConfigPathError, err)
	}

	return manager, nil
}

func (m *Manager) InventoryPath() string {
	return filepath.Join(m.ConfigPath, "inventory")
}

func (m *Manager) GetVM(name string) (Container, bool) {
	name = strings.Replace(name, ".", "-", -1)
	containers := m.containers()
	container, ok := containers[name]

	return container, ok
}

func (m *Manager) CreateVM(name string) error {
	container, err := m.newVM(name)
	if err != nil {
		return err
	}

	if err := container.CreateConfig(); err != nil {
		return err
	}

	configFile, err := os.Open(container.ConfigFile)
	if err != nil {
		return err
	}
	defer configFile.Close()

	version := m.trellis.CliConfig.Vm.Ubuntu

	launchCmd := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("lxc", []string{"launch", "ubuntu:" + version, container.Name})

	launchCmd.Stdin = configFile
	err = launchCmd.Run()

	if err != nil {
		return err
	}

	return postStart(m, &container)
}

func (m *Manager) DeleteVM(name string) error {
	container, ok := m.GetVM(name)

	if !ok {
		m.ui.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if container.Stopped() {
		return command.WithOptions(
			command.WithTermOutput(),
			command.WithLogging(m.ui),
		).Cmd("lxc", []string{"delete", container.Name}).Run()
	} else {
		return fmt.Errorf("Error: VM is running. Run `trellis vm stop` to stop it.")
	}
}

// TODO: set working dir to site path?
func (m *Manager) OpenShell(name string, commandArgs []string) error {
	container, ok := m.GetVM(name)

	if !ok {
		m.ui.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if container.Stopped() {
		m.ui.Info("VM is not running. Run `trellis vm start` to start it.")
		return nil
	}

	args := []string{"exec", container.Name, "--", "sh", "-c", fmt.Sprintf("%s", strings.Join(commandArgs, " "))}

	return command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("lxc", args).Run()
}

func (m *Manager) StartVM(name string) error {
	container, ok := m.GetVM(name)

	if !ok {
		return vm.VmNotFoundErr
	}

	if container.Running() {
		m.ui.Info(fmt.Sprintf("%s VM already running", color.GreenString("[✓]")))
		return nil
	}

	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("lxc", []string{"start", container.Name}).Run()

	if err != nil {
		return err
	}

	return postStart(m, &container)
}

func (m *Manager) StopVM(name string) error {
	container, ok := m.GetVM(name)

	if !ok {
		m.ui.Info("VM does not exist for this project. Run `trellis vm start` to create it.")
		return nil
	}

	if container.Stopped() {
		m.ui.Info(fmt.Sprintf("%s VM already stopped", color.GreenString("[✓]")))
		return nil
	}

	err := command.WithOptions(
		command.WithTermOutput(),
		command.WithLogging(m.ui),
	).Cmd("lxc", []string{"stop", container.Name}).Run()

	if err != nil {
		return fmt.Errorf("Error stopping VM\n%v", err)
	}

	if err = m.removeHosts(container); err != nil {
		return err
	}

	return nil
}

func (m *Manager) hydrateVM(container *Container) error {
	c, _ := m.GetVM(container.Name)
	*container = c

	return nil
}

// TODO: error handling
func (m *Manager) initVM(container *Container) {
	user, _ := user.Current()
	container.Username = user.Username
	container.Uid = user.Uid
	container.Gid = user.Gid
	container.ConfigFile = filepath.Join(m.ConfigPath, container.Name+".yml")
	container.InventoryFile = m.InventoryPath()
	container.Sites = m.Sites
}

func (m *Manager) newVM(name string) (Container, error) {
	name = strings.Replace(name, ".", "-", -1)
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return Container{}, err
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
		return Container{}, fmt.Errorf("No valid SSH public key found. Attempted paths: %s", strings.Join(defaultSshKeys, ", "))
	}

	container := Container{
		Name:         name,
		Devices:      devices,
		SshPublicKey: string(sshPublicKey),
	}

	m.initVM(&container)

	return container, nil
}

func (m *Manager) createConfigPath() error {
	return os.MkdirAll(m.ConfigPath, 0755)
}

func (m *Manager) addHosts(container Container) error {
	if err := container.CreateInventoryFile(); err != nil {
		return err
	}

	ip, err := container.IP()
	if err != nil {
		return err
	}

	if err := m.HostsResolver.AddHosts(container.Name, ip); err != nil {
		return err
	}

	return nil
}

func (m *Manager) containers() (containersByName map[string]Container) {
	containers := []Container{}
	containersByName = make(map[string]Container)

	output, _ := command.Cmd("lxc", []string{"list", "--format=json"}).Output()
	json.Unmarshal(output, &containers)

	for _, container := range containers {
		m.initVM(&container)
		containersByName[container.Name] = container
	}

	return containersByName
}

func (m *Manager) removeHosts(container Container) error {
	return m.HostsResolver.RemoveHosts(container.Name)
}

func (m *Manager) retryHydrateVM(container *Container, ctx context.Context) error {
	interval := 1 * time.Second

	for {
		err := m.hydrateVM(container)
		if err != nil {
			return err
		}

		_, err = container.IP()
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

func postStart(manager *Manager, container *Container) error {
	// Hydrate container with data from lxc that is only available after starting (mainly the IP)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := manager.retryHydrateVM(container, ctx)
	if err != nil {
		return err
	}

	if err = manager.addHosts(*container); err != nil {
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
