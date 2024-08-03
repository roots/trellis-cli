package lima

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/template"

	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

//go:embed files/config.yml
var ConfigTemplate string

//go:embed files/inventory.txt
var inventoryTemplate string

var (
	ConfigErr = errors.New("Could not write Lima config file")
	IpErr     = errors.New("Could not determine IP address for VM instance")
)

type PortForward struct {
	GuestPort int `yaml:"guestPort"`
	HostPort  int `yaml:"hostPort"`
}

type Image struct {
	Alias    string
	Location string `yaml:"location"`
	Arch     string `yaml:"arch"`
}

type Config struct {
	Images       []Image       `yaml:"images"`
	PortForwards []PortForward `yaml:"portForwards"`
}

type Instance struct {
	InventoryFile string
	Sites         map[string]*trellis.Site
	Name          string `json:"name"`
	Status        string `json:"status"`
	Dir           string `json:"dir"`
	Arch          string `json:"arch"`
	Cpus          int    `json:"cpus"`
	Memory        int    `json:"memory"`
	Disk          int    `json:"disk"`
	SshLocalPort  int    `json:"sshLocalPort,omitempty"`
	Config        Config `json:"config"`
	Username      string `json:"username,omitempty"`
}

func (i *Instance) ConfigFile() string {
	return filepath.Join(i.Dir, "lima.yaml")
}

func (i *Instance) GenerateConfig() (*bytes.Buffer, error) {
	var contents bytes.Buffer

	tpl := template.Must(template.New("lima").Parse(ConfigTemplate))

	if err := tpl.Execute(&contents, i); err != nil {
		return &contents, fmt.Errorf("%v: %w", ConfigErr, err)
	}

	return &contents, nil
}

func (i *Instance) UpdateConfig() error {
	contents, err := i.GenerateConfig()
	if err != nil {
		return err
	}

	if err := os.WriteFile(i.ConfigFile(), contents.Bytes(), 0666); err != nil {
		return fmt.Errorf("%v: %w", ConfigErr, err)
	}

	return nil
}

func (i *Instance) CreateInventoryFile() error {
	if i.SshLocalPort == 0 {
		return fmt.Errorf("SshLocalPort is not set. This is a trellis-cli bug.")
	}

	tpl := template.Must(template.New("lima").Parse(inventoryTemplate))

	file, err := os.Create(i.InventoryFile)
	if err != nil {
		return fmt.Errorf("Could not create Ansible inventory file: %v", err)
	}

	err = tpl.Execute(file, i)
	if err != nil {
		return fmt.Errorf("Could not template Ansible inventory file: %v", err)
	}

	return nil
}

/*
Gets the IP address of the instance using the output of `ip route`:
  default via 192.168.64.1 proto dhcp src 192.168.64.2 metric 100
  192.168.64.0/24 proto kernel scope link src 192.168.64.2
  192.168.64.1 proto dhcp scope link src 192.168.64.2 metric 100
*/
func (i *Instance) IP() (ip string, err error) {
	output, err := command.Cmd(
		"limactl",
		[]string{"shell", "--workdir", "/", i.Name, "ip", "route", "show", "dev", "lima0"},
	).CombinedOutput()

	if err != nil {
		return "", fmt.Errorf("%w: %v\n%s", IpErr, err, string(output))
	}

	re := regexp.MustCompile(`default via .* src ([0-9\.]+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("%w: no IP address could be matched in the ip route output\n%s", IpErr, string(output))
	}

	ip = matches[1]

	return ip, nil
}

func (i *Instance) Running() bool {
	return i.Status == "Running"
}

func (i *Instance) Stopped() bool {
	return i.Status == "Stopped"
}

func (i *Instance) getUsername() ([]byte, error) {
	user, err := command.Cmd("limactl", []string{"shell", i.Name, "whoami"}).Output()
	return user, err
}
