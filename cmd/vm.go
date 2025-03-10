package cmd

import (
	"os"
	"path/filepath"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

const VagrantInventoryFilePath string = ".vagrant/provisioners/ansible/inventory/vagrant_ansible_inventory"

func findDevInventory(trellis *trellis.Trellis, ui cli.Ui) string {
	// Use the newVmManager variable from main.go
	manager, managerErr := newVmManager(trellis, ui)

	if managerErr == nil {
		_, vmInventoryErr := os.Stat(manager.InventoryPath())
		if vmInventoryErr == nil {
			return manager.InventoryPath()
		}
	}

	if _, vagrantInventoryErr := os.Stat(filepath.Join(trellis.Path, VagrantInventoryFilePath)); vagrantInventoryErr == nil {
		return VagrantInventoryFilePath
	}

	return ""
}
