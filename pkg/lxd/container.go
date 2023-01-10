package lxd

import (
	_ "embed"
	"errors"
	"fmt"
	"os"
	"text/template"

	"github.com/roots/trellis-cli/trellis"
)

//go:embed files/config.yml
var ConfigTemplate string

//go:embed files/inventory.txt
var inventoryTemplate string

var (
	ConfigErr = errors.New("Could not write LXD config file")
	IpErr     = errors.New("Could not determine IP address for VM")
)

type Device struct {
	Source string
	Dest   string
}

type NetworkAddress struct {
	Family  string `json:"family"`
	Address string `json:"address"`
}

type Network struct {
	Addresses []NetworkAddress `json:"addresses"`
}

type State struct {
	Status  string             `json:"status"`
	Network map[string]Network `json:"network"`
}

type Container struct {
	ConfigFile    string
	InventoryFile string
	Sites         map[string]*trellis.Site
	Name          string `json:"name"`
	State         State  `json:"state"`
	Username      string `json:"username,omitempty"`
	Uid           string
	Gid           string
	SshPublicKey  string
	Devices       map[string]Device
}

func (c *Container) CreateConfig() error {
	tpl := template.Must(template.New("lxc").Parse(ConfigTemplate))

	file, err := os.Create(c.ConfigFile)
	if err != nil {
		return fmt.Errorf("%v: %w", ConfigErr, err)
	}

	err = tpl.Execute(file, c)
	if err != nil {
		return fmt.Errorf("%v: %w", ConfigErr, err)
	}

	return nil
}

func (c *Container) CreateInventoryFile() error {
	tpl := template.Must(template.New("lxd").Parse(inventoryTemplate))

	file, err := os.Create(c.InventoryFile)
	if err != nil {
		return fmt.Errorf("Could not create Ansible inventory file: %v", err)
	}

	err = tpl.Execute(file, c)
	if err != nil {
		return fmt.Errorf("Could not template Ansible inventory file: %v", err)
	}

	return nil
}

func (c *Container) IP() (ip string, err error) {
	network, ok := c.State.Network["eth0"]
	if !ok {
		return "", fmt.Errorf("%v: eth0 network not found", IpErr)
	}

	for _, address := range network.Addresses {
		if address.Family == "inet" && address.Address != "" {
			return address.Address, nil
		}
	}

	return "", fmt.Errorf("%v: inet address family not found", IpErr)
}

func (c *Container) Running() bool {
	return c.State.Status == "Running"
}

func (c *Container) Stopped() bool {
	return c.State.Status == "Stopped"
}
