package trellis

import (
	"bytes"
	"flag"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
)

// Tests based on
// https://github.com/mitchellh/cli/blob/5454ffe87bc5c6d8b6b21c825617755e18a07828/cli_test.go#L1125-L1225

// envComplete is the env var that the complete library sets to specify
// it should be calculating an auto-completion.
const envComplete = "COMP_LINE"

func TestCompletionFunctions(t *testing.T) {
	trellis := NewTrellis()

	defer TestChdir(t, "testdata/trellis")()

	if err := trellis.LoadProject(); err != nil {
		t.Fatalf(err.Error())
	}

	flags := flag.NewFlagSet("", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	flagsPredictor := complete.Flags{
		"--branch": complete.PredictNothing,
	}

	cases := []struct {
		Predictor complete.Predictor
		Completed []string
		Last      string
		Expected  []string
	}{
		{trellis.AutocompleteEnvironment(flags), []string{"deploy"}, "", []string{"development", "valet-link", "production"}},
		{trellis.AutocompleteEnvironment(flags), []string{"deploy"}, "d", []string{"development"}},
		{trellis.AutocompleteEnvironment(flags), []string{"deploy", "production"}, "", nil},
		{trellis.AutocompleteEnvironment(flags), []string{"deploy"}, "--b", []string{"--branch"}},
		{trellis.AutocompleteEnvironment(flags), []string{"deploy", "--branch=foo"}, "", []string{"development", "valet-link", "production"}},
		{trellis.AutocompleteEnvironment(flags), []string{"deploy", "--branch=foo"}, "pro", []string{"production"}},
		{trellis.AutocompleteSite(flags), []string{"deploy"}, "", []string{"development", "valet-link", "production"}},
		{trellis.AutocompleteSite(flags), []string{"deploy"}, "d", []string{"development"}},
		{trellis.AutocompleteSite(flags), []string{"deploy", "production"}, "", []string{"example.com"}},
		{trellis.AutocompleteSite(flags), []string{"deploy", "--branch=foo"}, "dev", []string{"development"}},
		{trellis.AutocompleteSite(flags), []string{"deploy", "--branch=foo", "production"}, "", []string{"example.com"}},
		{trellis.AutocompleteSite(flags), []string{"deploy", "--branch=foo", "production"}, "example", []string{"example.com"}},
	}

	for _, tc := range cases {
		t.Run(tc.Last, func(t *testing.T) {
			var flagValue string

			flags = flag.NewFlagSet("", flag.ContinueOnError)
			flags.StringVar(&flagValue, "branch", "", "Branch name")

			command := new(cli.MockCommandAutocomplete)
			command.AutocompleteArgsValue = tc.Predictor
			command.AutocompleteFlagsValue = flagsPredictor

			cli := &cli.CLI{
				Commands: map[string]cli.CommandFactory{
					"deploy": func() (cli.Command, error) { return command, nil },
				},
				Autocomplete: true,
			}

			// Setup the autocomplete line
			var input bytes.Buffer
			input.WriteString("cli ")
			if len(tc.Completed) > 0 {
				input.WriteString(strings.Join(tc.Completed, " "))
				input.WriteString(" ")
			}
			input.WriteString(tc.Last)
			defer testAutocomplete(t, input.String())()

			// Setup the output so that we can read it. We don't need to
			// reset os.Stdout because testAutocomplete will do that for us.
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatalf("err: %s", err)
			}
			defer r.Close() // Only defer reader since writer is closed below
			os.Stdout = w

			// Run
			exitCode, err := cli.Run()
			w.Close()
			if err != nil {
				t.Fatalf("err: %s", err)
			}

			if exitCode != 0 {
				t.Fatalf("bad: %d", exitCode)
			}

			// Copy the output and get the autocompletions. We trim the last
			// element if we have one since we usually output a final newline
			// which results in a blank.
			var outBuf bytes.Buffer
			io.Copy(&outBuf, r)
			actual := strings.Split(outBuf.String(), "\n")
			if len(actual) > 0 {
				actual = actual[:len(actual)-1]
			}
			if len(actual) == 0 {
				// If we have no elements left, make the value nil since
				// this is what we use in tests.
				actual = nil
			}

			sort.Strings(actual)
			sort.Strings(tc.Expected)

			if !reflect.DeepEqual(actual, tc.Expected) {
				t.Fatalf("\n\nExpected:\n%#v\n\nActual:\n%#v", tc.Expected, actual)
			}
		})
	}
}

// testAutocomplete sets up the environment to behave like a <tab> was
// pressed in a shell to autocomplete a command.
func testAutocomplete(t *testing.T, input string) func() {
	// This env var is used to trigger autocomplete
	os.Setenv(envComplete, input)

	// Change stdout/stderr since the autocompleter writes directly to them.
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	os.Stdout = w
	os.Stderr = w

	return func() {
		// Reset our env
		os.Unsetenv(envComplete)

		// Reset stdout, stderr
		os.Stdout = oldStdout
		os.Stderr = oldStderr

		// Close our pipe
		r.Close()
		w.Close()
	}
}
