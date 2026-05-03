package trust

import (
	"errors"
	"strings"
)

// fakeCall records one invocation for later assertion.
type fakeCall struct {
	Name  string
	Args  []string
	Stdin []byte
}

// fakeResponse is the canned answer for a matched command.
type fakeResponse struct {
	Stdout   []byte
	Stderr   []byte
	Err      error
	ExitCode int
}

// fakeRunner is a recording, scriptable runner. Tests register match
// functions that return a response when a call matches; calls are recorded
// in order so tests can assert on the sequence and arguments.
type fakeRunner struct {
	matches []fakeMatch
	missing map[string]bool
	calls   []fakeCall
}

type fakeMatch struct {
	pred     func(name string, args []string) bool
	response fakeResponse
}

// onArgs registers a response for the first call whose name+args satisfy the
// predicate. Each registration fires at most once. Tests typically call
// onArgs once per command they expect.
func (f *fakeRunner) onArgs(pred func(name string, args []string) bool, resp fakeResponse) {
	f.matches = append(f.matches, fakeMatch{pred: pred, response: resp})
}

// on is a convenience that matches by program name + the first n args being
// equal to the supplied prefix. e.g. on("security", "add-trusted-cert").
func (f *fakeRunner) on(name string, argsPrefix ...string) *fakeResponseBuilder {
	return &fakeResponseBuilder{
		runner: f,
		pred: func(n string, a []string) bool {
			if n != name {
				return false
			}
			if len(a) < len(argsPrefix) {
				return false
			}
			for i, want := range argsPrefix {
				if a[i] != want {
					return false
				}
			}
			return true
		},
	}
}

// fakeResponseBuilder lets tests chain a response onto a partial match.
type fakeResponseBuilder struct {
	runner *fakeRunner
	pred   func(name string, args []string) bool
}

func (b *fakeResponseBuilder) returns(resp fakeResponse) {
	b.runner.onArgs(b.pred, resp)
}

func (b *fakeResponseBuilder) succeed() {
	b.returns(fakeResponse{})
}

func (b *fakeResponseBuilder) succeedWith(stdout string) {
	b.returns(fakeResponse{Stdout: []byte(stdout)})
}

func (b *fakeResponseBuilder) failWith(stderr string) {
	b.returns(fakeResponse{Stderr: []byte(stderr), Err: errors.New("exit 1")})
}

// markMissing makes Lookup(name) return an error, simulating the binary
// not being on PATH.
func (f *fakeRunner) markMissing(name string) {
	if f.missing == nil {
		f.missing = map[string]bool{}
	}
	f.missing[name] = true
}

func (f *fakeRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	return f.run(nil, name, args)
}

func (f *fakeRunner) RunStdin(stdin []byte, name string, args ...string) ([]byte, []byte, error) {
	return f.run(stdin, name, args)
}

func (f *fakeRunner) run(stdin []byte, name string, args []string) ([]byte, []byte, error) {
	f.calls = append(f.calls, fakeCall{Name: name, Args: append([]string(nil), args...), Stdin: append([]byte(nil), stdin...)})
	for i, m := range f.matches {
		if m.pred(name, args) {
			// Consume single-shot match.
			f.matches = append(f.matches[:i], f.matches[i+1:]...)
			combined := make([]byte, 0, len(m.response.Stdout)+len(m.response.Stderr))
			combined = append(combined, m.response.Stdout...)
			combined = append(combined, m.response.Stderr...)
			return m.response.Stdout, combined, m.response.Err
		}
	}
	return nil, nil, errors.New("fakeRunner: no canned response for " + name + " " + strings.Join(args, " "))
}

func (f *fakeRunner) Lookup(name string) (string, error) {
	if f.missing[name] {
		return "", errors.New("not found")
	}
	return "/usr/bin/" + name, nil
}

// callCount returns how many recorded calls match the predicate.
func (f *fakeRunner) callCount(pred func(c fakeCall) bool) int {
	n := 0
	for _, c := range f.calls {
		if pred(c) {
			n++
		}
	}
	return n
}

// hasCall reports whether at least one recorded call matches name and a
// prefix of args.
func (f *fakeRunner) hasCall(name string, argsPrefix ...string) bool {
	return f.callCount(func(c fakeCall) bool {
		if c.Name != name || len(c.Args) < len(argsPrefix) {
			return false
		}
		for i, want := range argsPrefix {
			if c.Args[i] != want {
				return false
			}
		}
		return true
	}) > 0
}
