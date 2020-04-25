package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

type MockProject struct {
	detected bool
}

func (p *MockProject) Detect(path string) (projectPath string, ok bool) {
	return "trellis", p.detected
}

func mockExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprintf(os.Stdout, strings.Join(os.Args[3:], " "))
	os.Exit(0)
}

type MockCommand struct {
	cmd  string
	args string
	env  []string
}

type MockCommandExecutor struct {
	Command *MockCommand
}

func (m *MockCommandExecutor) Exec(argv0 string, argv []string, envv []string) (err error) {
	m.Command.cmd = argv0
	m.Command.args = strings.Join(argv, " ")
	m.Command.env = envv
	return nil
}

func (m *MockCommandExecutor) LookPath(file string) (string, error) {
	return file, nil
}

func MockExec(t *testing.T) func() {
	t.Helper()

	execCommand = mockExecCommand
	return func() { execCommand = exec.Command }
}
