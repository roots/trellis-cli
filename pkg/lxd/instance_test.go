package lxd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/roots/trellis-cli/command"
)

func TestCreateInventoryFile(t *testing.T) {
	dir := t.TempDir()

	expectedIP := "1.2.3.4"

	instance := &Instance{
		InventoryFile: filepath.Join(dir, "inventory"),
		Username:      "dev",
		State: State{
			Network: map[string]Network{
				"eth0": {
					Addresses: []NetworkAddress{{Address: expectedIP, Family: "inet"}}},
			},
		},
	}

	err := instance.CreateInventoryFile()
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(instance.InventoryFile)

	if err != nil {
		t.Fatal(err)
	}

	expected := fmt.Sprintf(`default ansible_host=%s ansible_user=dev ansible_ssh_common_args='-o StrictHostKeyChecking=no'

[development]
default

[web]
default
`, expectedIP)

	if string(content) != expected {
		t.Errorf("expected %s\ngot %s", expected, string(content))
	}
}

func TestIP(t *testing.T) {
	expectedIP := "10.99.30.5"

	instance := &Instance{
		Name: "test",
		State: State{
			Network: map[string]Network{
				"eth0": {
					Addresses: []NetworkAddress{{Address: expectedIP, Family: "inet"}}},
			},
		},
	}

	ip, err := instance.IP()
	if err != nil {
		t.Fatal(err)
	}

	if ip != expectedIP {
		t.Errorf("expected %s\ngot %s", expectedIP, ip)
	}
}

func TestCommandHelperProcess(t *testing.T) {
	command.CommandHelperProcess(t)
}
