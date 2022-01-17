package command

import (
	"io"
	"os"
	"os/exec"
	"testing"
)

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

func MockExec(t *testing.T, stdout io.Writer, stderr io.Writer) func() {
	t.Helper()

	Mock(MockExecCommand(stdout, stderr))

	return func() {
		Restore()
	}
}
