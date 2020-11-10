package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/mitchellh/cli"
)

var execCommandWithOutput = CommandExecWithOutput
var execCommand = CommandExec

type UiErrorWriter struct {
	Ui cli.Ui
}

func (w *UiErrorWriter) Write(p []byte) (n int, err error) {
	n = len(p)
	if n > 0 && p[n-1] == '\n' {
		p = p[:n-1]
	}

	w.Ui.Error(string(p))
	return n, nil
}

func CommandExecWithOutput(command string, args []string, ui cli.Ui) *exec.Cmd {
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = &UiErrorWriter{ui}
	cmd.Stdout = &cli.UiWriter{ui}

	ui.Info(fmt.Sprintf("Running command => %s", strings.Join(cmd.Args, " ")))

	return cmd
}

func CommandExecWithStderrOnly(command string, args []string, ui cli.Ui) *exec.Cmd {
	cmd := exec.Command(command, args...)
	cmd.Stderr = os.Stderr

	ui.Info(fmt.Sprintf("Running command => %s", strings.Join(cmd.Args, " ")))

	return cmd
}

func CommandExec(command string, args []string, ui cli.Ui) *exec.Cmd {
	cmd := exec.Command(command, args...)
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	ui.Info(fmt.Sprintf("Running command => %s", strings.Join(cmd.Args, " ")))

	return cmd
}
