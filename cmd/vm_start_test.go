package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/pkg/vm"
	"github.com/roots/trellis-cli/trellis"
)

// Store original functions to restore later
var (
	originalNewVmManager    = newVmManager
	originalNewProvisionCmd = NewProvisionCommand
)

// MockVmManager for testing
type MockVmManager struct {
	createCalled bool
	startCalled  bool
	siteName     string
}

func (m *MockVmManager) CreateInstance(name string) error {
	m.createCalled = true
	m.siteName = name
	return nil
}

func (m *MockVmManager) StartInstance(name string) error {
	m.startCalled = true
	m.siteName = name
	// First call returns VmNotFoundErr to trigger creation
	if !m.createCalled {
		return vm.VmNotFoundErr
	}
	return nil
}

func (m *MockVmManager) StopInstance(name string) error {
	return nil
}

func (m *MockVmManager) DeleteInstance(name string) error {
	return nil
}

// Add the missing InventoryPath method required by the vm.Manager interface
func (m *MockVmManager) InventoryPath() string {
	return "/mock/inventory/path"
}

// Update the OpenShell method with the correct signature
func (m *MockVmManager) OpenShell(sshUser string, hostName string, additionalArgs []string) error {
	// Mock implementation
	return nil
}

// Mock version of NewProvisionCommand for testing
type MockProvisionCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	Run     func(args []string) int
}

func (c *MockProvisionCommand) Synopsis() string { return "" }
func (c *MockProvisionCommand) Help() string     { return "" }

// Add a MockTrellis type that implements the GetVMInstanceName method
type MockTrellisWithVMName struct {
	*trellis.Trellis
	instanceName string
}

// Override the GetVMInstanceName method for testing
func (m *MockTrellisWithVMName) GetVMInstanceName() (string, error) {
	return m.instanceName, nil
}

func TestVmStartRunValidations(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	cases := []struct {
		name            string
		projectDetected bool
		args            []string
		out             string
		code            int
	}{
		{
			"no_project",
			false,
			nil,
			"No Trellis project detected",
			1,
		},
		{
			"too_many_args",
			true,
			[]string{"foo"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			vmStartCommand := NewVmStartCommand(ui, trellis)

			code := vmStartCommand.Run(tc.args)

			if code != tc.code {
				t.Errorf("expected code %d to be %d", code, tc.code)
			}

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}
		})
	}
}

func TestVmStartSavesInstanceName(t *testing.T) {
	cleanup := trellis.LoadFixtureProject(t)
	defer cleanup()

	// Setup test environment
	ui := cli.NewMockUi()
	mockTrellis := trellis.NewTrellis()
	mockTrellis.LoadProject()

	// Create command
	vmStartCommand := NewVmStartCommand(ui, mockTrellis)

	// Replace VM manager with mock
	mockManager := &MockVmManager{}

	// Save original function and replace with test double
	defer func() { newVmManager = originalNewVmManager }()
	newVmManager = func(t *trellis.Trellis, ui cli.Ui) (vm.Manager, error) {
		return mockManager, nil
	}

	// Mock provision command
	defer func() { NewProvisionCommand = originalNewProvisionCmd }()
	NewProvisionCommand = func(ui cli.Ui, trellis *trellis.Trellis) *ProvisionCommand {
		// Create an actual ProvisionCommand instead of trying to cast from MockProvisionCommand
		cmd := &ProvisionCommand{
			UI:      ui,
			Trellis: trellis,
		}

		// No need for type casting, return the real command
		return cmd
	}

	// Run command
	code := vmStartCommand.Run([]string{})

	// Check VM was created and started
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !mockManager.createCalled {
		t.Error("expected CreateInstance to be called")
	}
	if !mockManager.startCalled {
		t.Error("expected StartInstance to be called")
	}

	// Check instance file was created
	instancePath := filepath.Join(mockTrellis.ConfigPath(), "lima", "instance")
	data, err := os.ReadFile(instancePath)
	if err != nil {
		t.Errorf("expected instance file to exist: %v", err)
		return
	}

	instanceName := strings.TrimSpace(string(data))
	if instanceName != mockManager.siteName {
		t.Errorf("expected instance name %q, got %q", mockManager.siteName, instanceName)
	}
}

// Add this test to verify the VM name resolution
func TestVmStartUsesGetVMInstanceName(t *testing.T) {
	cleanup := trellis.LoadFixtureProject(t)
	defer cleanup()

	// Setup test environment with our custom mock
	ui := cli.NewMockUi()
	mockTrellis := trellis.NewTrellis()
	mockTrellis.LoadProject()

	// Create a custom mock Trellis that returns a specific instance name
	mockTrellisWithVMName := &MockTrellisWithVMName{
		Trellis:      mockTrellis,
		instanceName: "custom-instance-name",
	}

	// Create command with our custom mock
	vmStartCommand := NewVmStartCommand(ui, mockTrellisWithVMName.Trellis)

	// Replace VM manager with mock
	mockManager := &MockVmManager{}

	// Save original function and replace with test double
	defer func() { newVmManager = originalNewVmManager }()
	newVmManager = func(t *trellis.Trellis, ui cli.Ui) (vm.Manager, error) {
		return mockManager, nil
	}

	// Mock provision command
	defer func() { NewProvisionCommand = originalNewProvisionCmd }()
	NewProvisionCommand = func(ui cli.Ui, trellis *trellis.Trellis) *ProvisionCommand {
		cmd := &ProvisionCommand{
			UI:      ui,
			Trellis: trellis,
		}
		return cmd
	}

	// Run command
	code := vmStartCommand.Run([]string{})

	// Check VM was created and started with the correct instance name
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !mockManager.createCalled {
		t.Error("expected CreateInstance to be called")
	}
	if !mockManager.startCalled {
		t.Error("expected StartInstance to be called")
	}
	if mockManager.siteName != "custom-instance-name" {
		t.Errorf("expected site name to be 'custom-instance-name', got %s", mockManager.siteName)
	}

	// Check instance file was created with correct name
	instancePath := filepath.Join(mockTrellis.ConfigPath(), "lima", "instance")
	data, err := os.ReadFile(instancePath)
	if err != nil {
		t.Errorf("expected instance file to exist: %v", err)
		return
	}

	instanceName := strings.TrimSpace(string(data))
	if instanceName != "custom-instance-name" {
		t.Errorf("expected instance name %q, got %q", "custom-instance-name", instanceName)
	}
}
