package trellis

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetVMInstanceName(t *testing.T) {
	tempDir := t.TempDir()
	defer TestChdir(t, tempDir)()

	// Create a mock Trellis structure
	tp := &Trellis{
		ConfigDir: ".trellis",
		Path:      tempDir,
		Environments: map[string]*Config{
			"development": {
				WordPressSites: map[string]*Site{
					"example.com":      {},
					"another-site.com": {},
				},
			},
		},
	}

	// Create config directory
	if err := tp.CreateConfigDir(); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	// Test case 1: No instance file, no config setting
	// Should return the first site alphabetically (another-site.com)
	name, err := tp.GetVMInstanceName()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if name != "another-site.com" {
		t.Errorf("Expected 'another-site.com', got '%s'", name)
	}

	// Test case 2: With config setting
	tp.CliConfig.Vm.InstanceName = "custom-name"
	name, err = tp.GetVMInstanceName()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if name != "custom-name" {
		t.Errorf("Expected 'custom-name', got '%s'", name)
	}

	// Test case 3: With instance file (highest priority)
	limaDir := filepath.Join(tp.ConfigPath(), LimaDirName)
	if err := os.MkdirAll(limaDir, 0755); err != nil {
		t.Fatalf("Failed to create lima directory: %v", err)
	}
	instancePath := filepath.Join(limaDir, InstanceFile)
	if err := os.WriteFile(instancePath, []byte("instance-file-name"), 0644); err != nil {
		t.Fatalf("Failed to write instance file: %v", err)
	}

	name, err = tp.GetVMInstanceName()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if name != "instance-file-name" {
		t.Errorf("Expected 'instance-file-name', got '%s'", name)
	}

	// Clean up
	tp.CliConfig.Vm.InstanceName = ""
}

func TestSaveVMInstanceName(t *testing.T) {
	tempDir := t.TempDir()
	defer TestChdir(t, tempDir)()

	// Create a mock Trellis structure
	tp := &Trellis{
		ConfigDir: ".trellis",
		Path:      tempDir,
	}

	// Create config directory
	if err := tp.CreateConfigDir(); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	// Save instance name
	instanceName := "test-vm-instance"
	if err := tp.SaveVMInstanceName(instanceName); err != nil {
		t.Fatalf("Failed to save instance name: %v", err)
	}

	// Verify file was created
	instancePath := filepath.Join(tp.ConfigPath(), LimaDirName, InstanceFile)
	data, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatalf("Failed to read instance file: %v", err)
	}

	// Verify content
	if string(data) != instanceName {
		t.Errorf("Expected '%s', got '%s'", instanceName, string(data))
	}

	// Test updating existing file
	newInstanceName := "updated-name"
	if err := tp.SaveVMInstanceName(newInstanceName); err != nil {
		t.Fatalf("Failed to update instance name: %v", err)
	}

	// Verify update
	data, err = os.ReadFile(instancePath)
	if err != nil {
		t.Fatalf("Failed to read instance file: %v", err)
	}

	if string(data) != newInstanceName {
		t.Errorf("Expected '%s', got '%s'", newInstanceName, string(data))
	}
}

func TestReadInstanceNameFromFile(t *testing.T) {
	tempDir := t.TempDir()
	defer TestChdir(t, tempDir)()

	// Create a mock Trellis structure
	tp := &Trellis{
		ConfigDir: ".trellis",
		Path:      tempDir,
	}

	// Create config directory
	if err := tp.CreateConfigDir(); err != nil {
		t.Fatalf("Failed to create config directory: %v", err)
	}

	// Test reading non-existent file
	name, err := tp.readInstanceNameFromFile()
	if err == nil {
		t.Error("Expected error when reading non-existent file")
	}
	if name != "" {
		t.Errorf("Expected empty string, got '%s'", name)
	}

	// Create instance file
	limaDir := filepath.Join(tp.ConfigPath(), LimaDirName)
	if err := os.MkdirAll(limaDir, 0755); err != nil {
		t.Fatalf("Failed to create lima directory: %v", err)
	}
	instancePath := filepath.Join(limaDir, InstanceFile)
	expectedName := "instance-file-name"
	if err := os.WriteFile(instancePath, []byte(expectedName), 0644); err != nil {
		t.Fatalf("Failed to write instance file: %v", err)
	}

	// Test reading existing file
	name, err = tp.readInstanceNameFromFile()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if name != expectedName {
		t.Errorf("Expected '%s', got '%s'", expectedName, name)
	}

	// Test with trailing whitespace
	expectedName = "trimmed-name"
	if err := os.WriteFile(instancePath, []byte(expectedName+"\n"), 0644); err != nil {
		t.Fatalf("Failed to write instance file: %v", err)
	}

	name, err = tp.readInstanceNameFromFile()
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if name != expectedName {
		t.Errorf("Expected '%s', got '%s'", expectedName, name)
	}
}
