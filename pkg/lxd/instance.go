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

type Instance struct {
	ConfigFile    string
	InventoryFile string
	Sites         map[string]*trellis.Site
	Name          string `json:"name"`
	State         State  `json:"state"`
	Username      string `json:"username,omitempty"`
	Uid           int
	Gid           int
	SshPublicKey  string
	Devices       map[string]Device
}

func (i *Instance) CreateConfig() error {
	tpl := template.Must(template.New("lxc").Parse(ConfigTemplate))

	file, err := os.Create(i.ConfigFile)
	if err != nil {
		return fmt.Errorf("%v: %w", ConfigErr, err)
	}

	err = tpl.Execute(file, i)
	if err != nil {
		return fmt.Errorf("%v: %w", ConfigErr, err)
	}

	return nil
}

func (i *Instance) CreateInventoryFile() error {
	tpl := template.Must(template.New("lxd").Parse(inventoryTemplate))

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

func (i *Instance) IP() (ip string, err error) {
	network, ok := i.State.Network["eth0"]
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

func (i *Instance) Running() bool {
	return i.State.Status == "Running"
}

func (i *Instance) Stopped() bool {
	return i.State.Status == "Stopped"
}
