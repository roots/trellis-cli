package cmd

import (
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestDropletCreateRunValidations(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

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
		{
			"no_args",
			true,
			nil,
			"Error: missing arguments (expected exactly 1, got 0)",
			1,
		},
		{
			"too_many_args",
			true,
			[]string{"production", "foo"},
			"Error: too many arguments",
			1,
		},
		{
			"invalid_env",
			true,
			[]string{"foo"},
			"Error: foo is not a valid environment",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			dropletCreateCommand := NewDropletCreateCommand(ui, trellis)

			code := dropletCreateCommand.Run(tc.args)

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
