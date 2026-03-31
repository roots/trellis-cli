package output

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/roots/trellis-cli/pkg/ansible"
)

const ansibleCallbackEnvVar = "ANSIBLE_STDOUT_CALLBACK"
const jsonlCallback = "ansible.posix.jsonl"

// RunWithPrettifier runs an ansible-playbook command with pretty output.
// The writer parameter is used for output in both raw and pretty modes.
// It falls back to raw output when:
//   - The playbook has verbose mode enabled
//   - stdout is not a TTY (piped, CI, etc.)
//   - A JSON parse error occurs mid-stream
func RunWithPrettifier(cmd *exec.Cmd, playbook *ansible.Playbook, writer io.Writer) error {
	if playbook.Verbose || !isTTY() {
		return runRaw(cmd, writer)
	}

	return runPretty(cmd, writer)
}

func runRaw(cmd *exec.Cmd, writer io.Writer) error {
	cmd.Stdout = writer
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

func runPretty(cmd *exec.Cmd, _ io.Writer) error {
	cmd.Env = mergeEnv(os.Environ(), ansibleCallbackEnvVar, jsonlCallback)

	// Capture stderr to prevent deprecation warnings from corrupting spinner output.
	// Buffered stderr is displayed after the run completes.
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	// Run --list-tasks to get expected task counts for progress tracking.
	// Non-fatal if it fails — we just won't have an initial total.
	taskList := ListTasks(cmd.Path, cmd.Args[1:])

	pr, pw := io.Pipe()
	cmd.Stdout = pw

	// Pretty mode writes directly to os.Stdout for ANSI escape codes.
	// The writer param (cli.UiWriter) adds newlines that break cursor control.
	renderer := NewRenderer(os.Stdout, taskList)
	parser := NewParser(renderer, os.Stdout)

	done := make(chan error, 1)
	go func() {
		done <- parser.Parse(pr)
		pr.Close()
	}()

	cmdErr := cmd.Run()
	pw.Close()
	<-done

	// Display captured stderr (deprecation warnings, etc.) after clean output
	if stderrBuf.Len() > 0 {
		os.Stderr.Write(stderrBuf.Bytes())
	}

	return cmdErr
}

// mergeEnv returns a copy of env with key=value set or replaced.
func mergeEnv(env []string, key, value string) []string {
	prefix := key + "="
	result := make([]string, 0, len(env)+1)

	found := false
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			result = append(result, prefix+value)
			found = true
		} else {
			result = append(result, e)
		}
	}

	if !found {
		result = append(result, prefix+value)
	}

	return result
}

func isTTY() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}
