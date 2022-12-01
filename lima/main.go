package lima

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
	"gopkg.in/yaml.v2"
)

//go:embed files/config.yml
var ConfigTemplate string

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
	rand.Seed(time.Now().UnixNano())
	// TODO: we should check that the port is available first
	i.HttpForwardPort = rand.Intn(65534-60000) + 60000

	if err := i.CreateConfig(); err != nil {
		return err
	}

	args := []string{
		"start",
		"--name=" + i.Name,
		"--tty=false",
		i.ConfigFile,
	}

	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", args).Run()

	return err
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

func (i *Instance) Start() error {
	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", []string{"start", "--tty=false", i.Name}).Run()

	return err
}

func (i *Instance) Running() bool {
	return i.Status == "Running"
}

func (i *Instance) Stop() error {
	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", []string{"stop", i.Name}).Run()

	return err
}

func (i *Instance) Stopped() bool {
	return i.Status == "Stopped"
}

func (i *Instance) HttpHost() string {
	return fmt.Sprintf("http://127.0.0.1:%d", i.HttpForwardPort)
}

func (i *Instance) Hydrate() (err error) {
	if err = i.hydrateFromConfig(); err != nil {
		return err
	}
	if err = i.hydrateFromLima(); err != nil {
		return err
	}
	user, err := command.Cmd("limactl", []string{"shell", i.Name, "whoami"}).Output()
	if err == nil {
		i.Username = string(user)
	}

	return nil
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
