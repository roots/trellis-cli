package command

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mitchellh/cli"
)

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

type CommandOption func(*exec.Cmd)
type CommandFunc func(command string, args []string) *exec.Cmd
type ApplyOptionFunc func(option CommandOption, cmd *exec.Cmd)

var ExecCommand CommandFunc = execCommand
var OptionApplier ApplyOptionFunc = applyOption

type Command struct {
	options []CommandOption
}

func WithOptions(options ...CommandOption) *Command {
	return &Command{options: options}
}

func Cmd(command string, args []string) *exec.Cmd {
	cmd := ExecCommand(command, args)
	cmd.Stdin = os.Stdin

	return cmd
}

func (c *Command) Cmd(command string, args []string) *exec.Cmd {
	cmd := Cmd(command, args)

	for _, option := range c.options {
		OptionApplier(option, cmd)
	}

	return cmd
}

/*
  Enables mocking of the underlying ExecCommand command (defaults to exec.Command) and no-ops the OptionApplier function so they have no effect.
*/
func Mock(f CommandFunc) {
	OptionApplier = func(option CommandOption, cmd *exec.Cmd) {}
	ExecCommand = f
}

/*
  Restores the default ExecCommand and OptionApplier.
  Should be used via `defer` after `Mock` is called.
*/
func Restore() {
	OptionApplier = applyOption
	ExecCommand = execCommand
}

func WithUiOutput(ui cli.Ui) CommandOption {
	return func(cmd *exec.Cmd) {
		cmd.Stdout = &cli.UiWriter{Ui: ui}
		cmd.Stderr = &UiErrorWriter{ui}
	}
}

func WithCaptureOutput(stdout io.Writer, stderr io.Writer) CommandOption {
	return func(cmd *exec.Cmd) {
		cmd.Stdout = stdout
		cmd.Stderr = stderr
	}
}

func WithLogging(ui cli.Ui) CommandOption {
	return func(cmd *exec.Cmd) {
		ui.Info(fmt.Sprintf("Running command => %s", strings.Join(cmd.Args, " ")))
	}
}

func WithTermOutput() CommandOption {
	return func(cmd *exec.Cmd) {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
}

func applyOption(option CommandOption, cmd *exec.Cmd) {
	option(cmd)
}

func execCommand(command string, args []string) *exec.Cmd {
	return exec.Command(command, args...)
}
