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

func TestVmDeleteRunValidations(t *testing.T) {
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
			vmDeleteCommand := NewVmDeleteCommand(ui, trellis)

			code := vmDeleteCommand.Run(tc.args)

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

func TestVmDeleteRemovesInstanceFile(t *testing.T) {
	cleanup := trellis.LoadFixtureProject(t)
	defer cleanup()
	
	// Setup test environment
	ui := cli.NewMockUi()
	mockTrellis := trellis.NewTrellis()
	mockTrellis.LoadProject()
	
	// Create the lima directory and instance file
	limaDir := filepath.Join(mockTrellis.ConfigPath(), "lima")
	err := os.MkdirAll(limaDir, 0755)
	if err != nil {
		t.Fatalf("failed to create lima directory: %v", err)
	}
	
	instancePath := filepath.Join(limaDir, "instance")
	err = os.WriteFile(instancePath, []byte("example.com"), 0644)
	if err != nil {
		t.Fatalf("failed to write instance file: %v", err)
	}
	
	// Verify file exists before test
	if _, err := os.Stat(instancePath); os.IsNotExist(err) {
		t.Fatalf("failed to create test instance file")
	}
	
	// Create command
	vmDeleteCommand := NewVmDeleteCommand(ui, mockTrellis)
	vmDeleteCommand.force = true // Skip confirmation prompt
	
	// Replace VM manager with mock
	mockManager := &MockVmManager{}
	
	// Save original function and restore after test
	originalManagerFunc := newVmManager
	defer func() { newVmManager = originalManagerFunc }()
	
	newVmManager = func(t *trellis.Trellis, ui cli.Ui) (vm.Manager, error) {
		return mockManager, nil
	}
	
	// Run command
	code := vmDeleteCommand.Run([]string{})
	
	// Check command succeeded
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	
	// Check instance file was removed
	_, err = os.Stat(instancePath)
	if !os.IsNotExist(err) {
		t.Error("expected instance file to be deleted")
	}
}
