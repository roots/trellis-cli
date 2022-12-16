package lima

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
	"gopkg.in/yaml.v2"
)

//go:embed files/config.yml
var ConfigTemplate string

//go:embed files/inventory.txt
var inventoryTemplate string

var (
	ConfigError    = errors.New("Could not write Lima config file")
	HydrationError = errors.New("Could not fetch Lima instance data")
)

type PortForward struct {
	GuestPort int `yaml:"guestPort"`
	HostPort  int `yaml:"hostPort"`
}

type Config struct {
	PortForwards []PortForward `yaml:"portForwards"`
}

type Networkable interface {
	HttpHost() string
	IP() (string, error)
}

type Instance struct {
	ConfigPath      string
	ConfigFile      string
	Sites           map[string]*trellis.Site
	Name            string `json:"name"`
	Status          string `json:"status"`
	Dir             string `json:"dir"`
	Arch            string `json:"arch"`
	Cpus            int    `json:"cpus"`
	Memory          int    `json:"memory"`
	Disk            int    `json:"disk"`
	SshLocalPort    int    `json:"sshLocalPort"`
	HttpForwardPort int
	Username        string
}

func (i *Instance) Create() error {
	httpForwardPort, err := findFreeTCPLocalPort()
	if err != nil {
		return fmt.Errorf("Could not find a local free port for HTTP forwarding: %v", err)
	}

	i.HttpForwardPort = httpForwardPort

	if err := i.CreateConfig(); err != nil {
		return err
	}

	args := []string{
		"start",
		"--name=" + i.Name,
		"--tty=false",
		i.ConfigFile,
	}

	return command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", args).Run()
}

func (i *Instance) CreateConfig() error {
	tpl := template.Must(template.New("lima").Parse(ConfigTemplate))

	file, err := os.Create(i.ConfigFile)
	if err != nil {
		return fmt.Errorf("%v: %w", ConfigError, err)
	}

	err = tpl.Execute(file, i)
	if err != nil {
		return fmt.Errorf("%v: %w", ConfigError, err)
	}

	return nil
}

func (i *Instance) CreateInventoryFile() error {
	tpl := template.Must(template.New("lima").Parse(inventoryTemplate))

	file, err := os.Create(filepath.Join(i.ConfigPath, "inventory"))
	if err != nil {
		return fmt.Errorf("Could not create Ansible inventory file: %v", err)
	}

	err = tpl.Execute(file, i)
	if err != nil {
		return fmt.Errorf("Could not template Ansible inventory file: %v", err)
	}

	return nil
}

func (i *Instance) Delete() error {
	return command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", []string{"delete", i.Name}).Run()
}

// TODO Needs better error handling
func (i *Instance) IP() (ip string, err error) {
	output, err := command.Cmd(
		"limactl",
		[]string{"shell", "--workdir", "/", i.Name, "ip", "route", "show", "dev", "lima0"},
	).CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("Could not find IP address for VM instance %s", i.Name)
	}

	re := regexp.MustCompile(`([0-9\.]+)`)
	ip = re.FindString(string(output))

	if ip == "" {
		return ip, fmt.Errorf("Could not find IP address for VM instance %s", i.Name)
	} else {
		return ip, nil
	}
}

func (i *Instance) Running() bool {
	return i.Status == "Running"
}

func (i *Instance) Shell(commandArgs []string) error {
	args := []string{"shell", i.Name}
	args = append(args, commandArgs...)

	return command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", args).Run()
}

func (i *Instance) Start() error {
	return command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", []string{"start", "--tty=false", i.Name}).Run()
}

func (i *Instance) Stop() error {
	return command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", []string{"stop", i.Name}).Run()
}

func (i *Instance) Stopped() bool {
	return i.Status == "Stopped"
}

func (i *Instance) HttpHost() string {
	return fmt.Sprintf("http://127.0.0.1:%d", i.HttpForwardPort)
}

// TODO: replace when new `lima inspect` command is available
func (i *Instance) Hydrate(hydrateUser bool) (err error) {
	if err = i.hydrateFromConfig(); err != nil {
		return err
	}
	if err = i.hydrateFromLima(); err != nil {
		return err
	}

	if hydrateUser {
		user, err := i.getUsername()
		if err == nil {
			i.Username = string(user)
		}
	}

	return nil
}

func (i *Instance) getUsername() ([]byte, error) {
	user, err := command.Cmd("limactl", []string{"shell", i.Name, "whoami"}).Output()
	return user, err
}

func (i *Instance) hydrateFromConfig() error {
	config := &Config{}

	configYaml, err := os.ReadFile(i.ConfigFile)
	if err != nil {
		return nil
	}

	if err = yaml.Unmarshal(configYaml, config); err != nil {
		return fmt.Errorf("%v: %w", HydrationError, err)
	}

	i.HttpForwardPort = config.PortForwards[0].HostPort

	return nil
}

func (i *Instance) hydrateFromLima() error {
	output, err := command.Cmd("limactl", []string{"list", "--json", i.Name}).Output()
	if err != nil {
		return fmt.Errorf("%v: %w", HydrationError, err)
	}

	data := strings.Split(string(output), "\n")[0]

	if err = json.Unmarshal([]byte(data), i); err != nil {
		return fmt.Errorf("%v: %w", HydrationError, err)
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
