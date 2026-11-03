package lima

import (
	"bytes"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

type Network struct {
	VZNAT      bool   `json:"vzNAT,omitempty"`
	MACAddress string `json:"macAddress,omitempty"`
	Interface  string `json:"interface,omitempty"`
}

type Instance struct {
	InventoryFile string
	Sites         map[string]*trellis.Site
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	Dir           string    `json:"dir"`
	Arch          string    `json:"arch"`
	Cpus          int       `json:"cpus"`
	Memory        int       `json:"memory"`
	Disk          int       `json:"disk"`
	SshLocalPort  int       `json:"sshLocalPort,omitempty"`
	VMType        string    `json:"vmType,omitempty"`
	MountType     string    `json:"-"`
	Config        Config    `json:"config"`
	Network       []Network `json:"network,omitempty"`
	Username      string    `json:"username,omitempty"`
}

func (i *Instance) ConfigFile() string {
	return filepath.Join(i.Dir, "lima.yaml")
}

// ResolvedMountType returns the configured mount type, falling back to the
// default for the VM type (virtiofs for vz, 9p for qemu).
func (i *Instance) ResolvedMountType() string {
	if i.MountType != "" {
		return i.MountType
	}

	if i.VMType == "vz" {
		return "virtiofs"
	}

	return "9p"
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
	args := []string{"shell", "--workdir", "/", i.Name, "ip", "route", "show", "dev", "lima0"}
	if i.VMType == "qemu" {
		args = []string{"shell", "--workdir", "/", i.Name, "ip", "route", "show"}
	}

	output, err := command.Cmd("limactl", args).CombinedOutput()

	// Lima's cloud-init config renames the vzNAT interface to lima0, but on some
	// images (e.g. Ubuntu 26.04) the rename fails on first boot and the
	// interface keeps its predictable name (enp0s2). Find it by MAC instead.
	if err != nil && i.VMType != "qemu" && strings.Contains(string(output), `Cannot find device "lima0"`) {
		if name := i.interfaceByMAC(); name != "" {
			args = []string{"shell", "--workdir", "/", i.Name, "ip", "route", "show", "dev", name}
			output, err = command.Cmd("limactl", args).CombinedOutput()
		}
	}

	if err != nil {
		return "", fmt.Errorf("%w: %v\n%s", IpErr, err, string(output))
	}

	if i.VMType == "qemu" {
		reCustom := regexp.MustCompile(`src (192\.168\.56\.[0-9]+)`)
		customMatches := reCustom.FindStringSubmatch(string(output))
		if len(customMatches) >= 2 {
			return customMatches[1], nil
		}
	}

	re := regexp.MustCompile(`default via .* src ([0-9\.]+)`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return "", fmt.Errorf("%w: no IP address could be matched in the ip route output\n%s", IpErr, string(output))
	}

	return matches[1], nil
}

/*
Gets the guest interface name for the lima0 network by its MAC address using
the output of `ip -o link show`:

	3: enp0s2: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 ... link/ether 52:55:55:2f:cb:a6 brd ff:ff:ff:ff:ff:ff
*/
func (i *Instance) interfaceByMAC() string {
	mac := ""
	for _, n := range i.Network {
		if n.Interface == "lima0" && n.MACAddress != "" {
			mac = n.MACAddress
			break
		}
	}

	if mac == "" {
		return ""
	}

	output, err := command.Cmd("limactl", []string{"shell", "--workdir", "/", i.Name, "ip", "-o", "link", "show"}).Output()
	if err != nil {
		return ""
	}

	re := regexp.MustCompile(`(?mi)^\d+:\s+([^:@\s]+)[:@].*link/ether\s+` + regexp.QuoteMeta(mac) + `\b`)
	matches := re.FindStringSubmatch(string(output))
	if len(matches) < 2 {
		return ""
	}

	return matches[1]
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
