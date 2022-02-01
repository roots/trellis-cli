package command

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type MockCommand struct {
	Command  string   `json:"command"`
	Args     []string `json:"args"`
	Output   string   `json:"output"`
	ExitCode int      `json:"exit_code"`
}

func MockExecCommand(stdout io.Writer, stderr io.Writer) func(command string, args []string) *exec.Cmd {
	return func(command string, args []string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)

		cmd.Stdout = stdout
		cmd.Stderr = stderr
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}
}

func MockExecCommands(t *testing.T, commands []MockCommand) func() {
	t.Helper()

	tmpDir := t.TempDir()

	serializedCommands, err := json.Marshal(commands)
	if err != nil {
		t.Fatalf("error serializing commands: %s", err)
	}

	mockExecCommand := func(command string, args []string) *exec.Cmd {
		cs := []string{"-test.run=TestCommandHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{
			"GO_WANT_HELPER_PROCESS=1", fmt.Sprintf("GO_TEST_HELPER_TMP_PATH=%s", tmpDir),
			"GO_TEST_HELPER_COMMANDS=" + string(serializedCommands),
		}
		return cmd
	}

	Mock(mockExecCommand)

	return func() {
		Restore()
	}
}

func CommandHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	commands := []MockCommand{}
	err := json.Unmarshal([]byte(os.Getenv("GO_TEST_HELPER_COMMANDS")), &commands)
	if err != nil {
		t.Fatalf("error unmarshaling commands: %s", err)
	}

	command := strings.Join(os.Args[3:len(os.Args)], " ")

	commandExecuted := MockCommand{}
	commandFound := false

	for _, cmd := range commands {
		execCmd := exec.Command(cmd.Command, cmd.Args...)
		if execCmd.String() == command {
			commandExecuted = cmd
			commandFound = true
			break
		}
	}

	if !commandFound {
		t.Fatalf("command not found: %s\nmocked commands: %v", command, commands)
	}

	fmt.Fprintf(os.Stdout, commandExecuted.Output)
	os.Exit(commandExecuted.ExitCode)
}
