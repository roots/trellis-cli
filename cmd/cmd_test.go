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
