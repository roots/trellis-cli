package lima

import (
	_ "embed"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/roots/trellis-cli/command"
	"gopkg.in/yaml.v2"
)

//go:embed files/config.yml
var ConfigTemplate string

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
	SiteName        string
	Name            string `json:"name"`
	Status          string `json:"status"`
	Dir             string `json:"dir"`
	Arch            string `json:"arch"`
	Cpus            int    `json:"cpus"`
	Memory          int    `json:"memory"`
	Disk            int    `json:"disk"`
	SshLocalPort    int    `json:"sshLocalPort"`
	HttpForwardPort int
}

func NewInstance(siteName string, configPath string) *Instance {
	name := ConvertToInstanceName(siteName)

	instance := &Instance{
		SiteName:   siteName,
		Name:       name,
		ConfigPath: configPath,
		ConfigFile: filepath.Join(configPath, name+".yml"),
	}

	return instance
}

func HydrateInstance(siteName string, configPath string) (*Instance, error) {
	instance := NewInstance(siteName, configPath)

	if err := instance.hydrateFromConfig(); err != nil {
		return nil, err
	}
	if err := instance.hydrateFromLima(); err != nil {
		return nil, err
	}

	return instance, nil
}

func InstanceExists(name string) bool {
	name = ConvertToInstanceName(name)
	_, ok := GetInstance(name)

	return ok
}

func (i *Instance) Create() error {
	rand.Seed(time.Now().UnixNano())
	// TODO: we should check that the port is available first
	i.HttpForwardPort = rand.Intn(65534-60000) + 60000

	if err := i.CreateConfig(); err != nil {
		return err
	}

	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", []string{"start", "--tty=false", i.ConfigFile}).Run()

	return err
}

func (i *Instance) Start() error {
	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", []string{"start", "--tty=false", i.Name}).Run()

	return err
}

func (i *Instance) Stop() error {
	err := command.WithOptions(
		command.WithTermOutput(),
	).Cmd("limactl", []string{"stop", i.Name}).Run()

	return err
}

func (i *Instance) hydrateFromConfig() error {
	config := &Config{}

	configYaml, err := os.ReadFile(i.ConfigFile)
	if err != nil {
		return err
	}

	if err = yaml.Unmarshal(configYaml, config); err != nil {
		return err
	}

	i.HttpForwardPort = config.PortForwards[0].HostPort

	return nil
}

func (i *Instance) hydrateFromLima() error {
	output, err := command.Cmd("limactl", []string{"list", "--json", i.Name}).Output()
	if err != nil {
		return err
	}

	data := strings.Split(string(output), "\n")[0]

	if err = json.Unmarshal([]byte(data), i); err != nil {
		return err
	}

	return nil
}

func ConvertToInstanceName(value string) string {
	return strings.ReplaceAll(value, ".", "-")
}

func Instances() (instances map[string]Instance) {
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

func GetInstance(name string) (Instance, bool) {
	instances := Instances()
	instance, ok := instances[ConvertToInstanceName(name)]

	return instance, ok
}

func (i *Instance) CreateConfig() error {
	tpl := template.Must(template.New("lima").Parse(ConfigTemplate))

	file, err := os.Create(i.ConfigFile)
	if err != nil {
		return err
	}

	err = tpl.Execute(file, i)
	if err != nil {
		return err
	}

	return nil
}
