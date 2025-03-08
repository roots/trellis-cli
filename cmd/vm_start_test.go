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
	originalNewVmManager := newVmManager
	mockManager := &MockVmManager{}
	newVmManager = func(t *trellis.Trellis, ui cli.Ui) (vm.Manager, error) {
		return mockManager, nil
	}
	defer func() { newVmManager = originalNewVmManager }()
	
	// Mock provision command to return success
	originalNewProvisionCommand := NewProvisionCommand
	NewProvisionCommand = func(ui cli.Ui, trellis *trellis.Trellis) *ProvisionCommand {
		cmd := &ProvisionCommand{UI: ui, Trellis: trellis}
		cmd.Run = func(args []string) int {
			return 0
		}
		return cmd
	}
	defer func() { NewProvisionCommand = originalNewProvisionCommand }()
	
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
	}
	
	instanceName := strings.TrimSpace(string(data))
	if instanceName != mockManager.siteName {
		t.Errorf("expected instance name %q, got %q", mockManager.siteName, instanceName)
	}
}
