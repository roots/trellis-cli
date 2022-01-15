package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
)

func mockExecCommand(stdout io.Writer, stderr io.Writer) func(command string, args []string) *exec.Cmd {
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

	command.Mock(mockExecCommand(stdout, stderr))

	return func() {
		command.Restore()
	}
}

func MockUiExec(t *testing.T, ui *cli.MockUi) func() {
	t.Helper()

	command.Mock(mockExecCommand(ui.OutputWriter, ui.ErrorWriter))

	return func() {
		command.Restore()
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprintf(os.Stdout, strings.Join(os.Args[3:], " "))
	os.Exit(0)
}
