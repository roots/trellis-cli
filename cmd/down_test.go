package cmd

import (
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestDownRunValidations(t *testing.T) {
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
			"too_many_args",
			true,
			[]string{"foo"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			downCommand := &DownCommand{ui, trellis}

			code := downCommand.Run(tc.args)

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

func TestDownRun(t *testing.T) {
	trellis := trellis.NewMockTrellis(true)

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"default",
			[]string{},
			"vagrant halt",
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			downCommand := &DownCommand{ui, trellis}
			code := downCommand.Run(tc.args)

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
