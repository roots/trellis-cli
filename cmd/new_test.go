package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestNewRunValidations(t *testing.T) {
	cases := []struct {
		name            string
		projectDetected bool
		args            []string
		out             string
		code            int
	}{
		{
			"no_args",
			false,
			nil,
			"Error: missing arguments (expected exactly 1, got 0)",
			1,
		},
		{
			"too_many_args",
			false,
			[]string{"foo", "bar"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			mockProject := &MockProject{tc.projectDetected}
			trellis := trellis.NewTrellis(mockProject)
			newCommand := NewNewCommand(ui, trellis, "1.0.0")

			code := newCommand.Run(tc.args)

			if code != tc.code {
				t.Errorf("expected code %d to be %d", code, tc.code)
			}

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}
		})
	}
}

func TestAskDomain(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		askOutput string
		hostInput string
		domain    string
		err       string
	}{
		{
			"relative_path_with_domain",
			"example.com",
			"example.com",
			"example.com\n",
			"example.com",
			"",
		},
		{
			"strips_www_trd",
			"www.example.com",
			"example.com",
			"example.com\n",
			"example.com",
			"",
		},
		{
			"complex_relative_path_with_domain",
			"../foo/example.com",
			"example.com",
			"example.com\n",
			"example.com",
			"",
		},
		{
			"custom_input",
			"../foo/example.com",
			"example.com",
			"foobar.com\n",
			"foobar.com",
			"",
		},
		{
			"path_with_non_domain",
			"notadomain",
			"",
			"\n",
			"",
			"path `notadomain` must be a valid domain name",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			ui.InputReader = bytes.NewBuffer([]byte(tc.hostInput))
			domain, err := askDomain(ui, tc.path)
			askOutput := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(askOutput, tc.askOutput) {
				t.Errorf("expected ask output %q to contain %q", askOutput, tc.askOutput)
			}

			if domain != tc.domain {
				t.Errorf("expected domain %q to equal %q", domain, tc.domain)
			}

			if err != nil && !strings.Contains(err.Error(), tc.err) {
				t.Errorf("expected error %q to equal %q", err, tc.err)
			}
		})
	}
}
