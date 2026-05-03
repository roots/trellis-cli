package trust

import (
	"bytes"
	"os/exec"
)

// runner abstracts process execution so platform stores can be tested
// without spawning real binaries. The default implementation wraps
// os/exec; tests inject a fake.
type runner interface {
	// Run executes name with args. stdout is the standard output only;
	// combined is stdout + stderr merged. Both are populated even when
	// err is non-nil so callers can grep stderr for tool-specific
	// "not found" / "already trusted" messages.
	Run(name string, args ...string) (stdout []byte, combined []byte, err error)

	// RunStdin executes name with args, feeding stdin to the process.
	// Same return semantics as Run.
	RunStdin(stdin []byte, name string, args ...string) (stdout []byte, combined []byte, err error)

	// Lookup is the equivalent of exec.LookPath. Returns the absolute
	// path to name on PATH, or an error when not present.
	Lookup(name string) (string, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return runExec(nil, name, args...)
}

func (execRunner) RunStdin(stdin []byte, name string, args ...string) ([]byte, []byte, error) {
	return runExec(stdin, name, args...)
}

func (execRunner) Lookup(name string) (string, error) {
	return exec.LookPath(name)
}

func runExec(stdin []byte, name string, args ...string) ([]byte, []byte, error) {
	cmd := exec.Command(name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	stdout := stdoutBuf.Bytes()
	combined := make([]byte, 0, len(stdout)+stderrBuf.Len())
	combined = append(combined, stdout...)
	combined = append(combined, stderrBuf.Bytes()...)
	return stdout, combined, err
}
