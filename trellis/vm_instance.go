package trellis

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	LimaDirName     = "lima"
	InstanceFile    = "instance"
)

// GetVMInstanceName returns the VM instance name based on the following priority:
// 1. Instance file in .trellis/lima/instance
// 2. CliConfig instance_name setting
// 3. First site in development environment's wordpress_sites.yml
func (t *Trellis) GetVMInstanceName() (string, error) {
	// 1. Check for instance file
	instanceName, err := t.readInstanceNameFromFile()
	if err == nil && instanceName != "" {
		return instanceName, nil
	}

	// 2. Check CLI config for instance_name
	if t.CliConfig.Vm.InstanceName != "" {
		return t.CliConfig.Vm.InstanceName, nil
	}

	// 3. Simply use the first site in the development environment
	config := t.Environments["development"]
	if config == nil || len(config.WordPressSites) == 0 {
		return "", nil
	}
	
	// Get the first site name alphabetically (which is the default behavior)
	siteNames := t.SiteNamesFromEnvironment("development")
	if len(siteNames) > 0 {
		return siteNames[0], nil
	}
	
	return "", nil
}

// SaveVMInstanceName writes the VM instance name to the instance file
func (t *Trellis) SaveVMInstanceName(instanceName string) error {
	limaDir := filepath.Join(t.ConfigPath(), LimaDirName)
	
	// Create the lima directory if it doesn't exist
	if err := os.MkdirAll(limaDir, 0755); err != nil && !os.IsExist(err) {
		return err
	}
	
	instancePath := filepath.Join(limaDir, InstanceFile)
	return os.WriteFile(instancePath, []byte(instanceName), 0644)
}

// readInstanceNameFromFile reads the VM instance name from the instance file
func (t *Trellis) readInstanceNameFromFile() (string, error) {
	instancePath := filepath.Join(t.ConfigPath(), LimaDirName, InstanceFile)
	
	data, err := os.ReadFile(instancePath)
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(data)), nil
}
