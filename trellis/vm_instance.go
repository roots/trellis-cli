package trellis

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	LimaDirName  = "lima"
	InstanceFile = "instance"
)

// GetVMInstanceName returns the VM instance name based on the following priority:
// 1. Instance file in .trellis/lima/instance
// 2. CliConfig instance_name setting
// 3. Check for existing VMs matching any site name
// 4. First site in development environment's wordpress_sites.yml
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

	// 3. NEW: Check for existing VMs matching site names
	// Get all site names from the development environment
	siteNames := t.SiteNamesFromEnvironment("development")

	// Check if any of these site names already exists as a VM
	for _, siteName := range siteNames {
		vmExists, _ := checkVMExists(siteName)
		if vmExists {
			// Found existing VM - save this for future use
			t.SaveVMInstanceName(siteName)
			return siteName, nil
		}
	}

	// 4. Simply use the first site in the development environment
	config := t.Environments["development"]
	if config == nil || len(config.WordPressSites) == 0 {
		return "", nil
	}

	// Get the first site name alphabetically (which is the default behavior)
	if len(siteNames) > 0 {
		return siteNames[0], nil
	}

	return "", nil
}

// checkVMExists checks if a VM with the given name already exists
func checkVMExists(name string) (bool, error) {
	cmd := exec.Command("limactl", "list", "--json")
	output, err := cmd.Output()

	if err != nil {
		return false, err
	}

	// Simple string check rather than parsing JSON
	return strings.Contains(string(output), `"name":"`+name+`"`) ||
		strings.Contains(string(output), `"name": "`+name+`"`), nil
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
