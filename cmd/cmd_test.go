package cmd

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/command"
)

func MockUiExec(t *testing.T, ui *cli.MockUi) func() {
	t.Helper()

	command.Mock(command.MockExecCommand(ui.OutputWriter, ui.ErrorWriter))

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
