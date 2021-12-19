package cmd

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestExecRunValidations(t *testing.T) {
	cases := []struct {
		name            string
		projectDetected bool
		args            []string
		out             string
		code            int
	}{
		{
			"no_project",
			false,
			nil,
			"No Trellis project detected",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			mockProject := &MockProject{tc.projectDetected}
			trellis := trellis.NewTrellis(mockProject)
			execCommand := &ExecCommand{ui, trellis}

			code := execCommand.Run(tc.args)

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
