package command

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func fakeOption() CommandOption {
	return func(cmd *exec.Cmd) {
		cmd.Args = append(cmd.Args, "arg1")
	}
}

func anotherFakeOption() CommandOption {
	return func(cmd *exec.Cmd) {
		cmd.Args = append(cmd.Args, "arg2")
	}
}

func TestCmdWithOptions(t *testing.T) {
	cmd := WithOptions(fakeOption(), anotherFakeOption()).Cmd("foo", []string{})

	expected := fmt.Sprintf("foo arg1 arg2")
	actual := cmd.String()

	if actual != expected {
		t.Errorf("expected command: %s, got: %s", expected, actual)
	}
}

func TestMockAndRestore(t *testing.T) {
	path := "mocked"
	arg := "mocked_arg"
	expected := fmt.Sprintf("%s %s", path, arg)

	f := func(command string, args []string) *exec.Cmd { return exec.Command(path, arg) }
	Mock(f)

	cmd := WithOptions(fakeOption(), anotherFakeOption()).Cmd("foo", []string{})
	actual := cmd.String()

	if actual != expected {
		t.Errorf("expected command: %s, got: %s", expected, actual)
	}

	Restore()

	cmd = WithOptions(fakeOption()).Cmd("foo", []string{})
	expected = "foo arg1"
	actual = cmd.String()

	if actual != expected {
		t.Errorf("expected command: %s, got: %s", expected, actual)
	}
}

func TestWithUiOutput(t *testing.T) {
	ExecCommand = MockExecCommand(os.Stdout, os.Stderr)
	defer Restore()

	ui := cli.NewMockUi()
	WithOptions(WithUiOutput(ui)).Cmd("foo", []string{"arg"}).Run()

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

	if !strings.Contains(combined, "foo arg") {
		t.Errorf("expected command: %s, got: %s", "foo arg", combined)
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprintf(os.Stdout, strings.Join(os.Args[3:], " "))
	os.Exit(0)
}
