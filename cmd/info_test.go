package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestInfoRunValidations(t *testing.T) {
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
			tr := trellis.NewMockTrellis(tc.projectDetected)
			infoCommand := NewInfoCommand(ui, tr)

			code := infoCommand.Run(tc.args)

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

func TestInfoRun(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	ui := cli.NewMockUi()
	tr := trellis.NewTrellis()

	infoCommand := NewInfoCommand(ui, tr)
	code := infoCommand.Run([]string{})

	if code != 0 {
		t.Errorf("expected code 0, got %d\nError: %s", code, ui.ErrorWriter.String())
	}

	output := ui.OutputWriter.String()

	requiredStrings := []string{
		"Project:",
		"Virtualenv:",
		"VM:",
		"example.com",
	}

	for _, s := range requiredStrings {
		if !strings.Contains(output, s) {
			t.Errorf("expected output to contain %q\nGot: %s", s, output)
		}
	}
}

func TestInfoRunJSON(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	ui := cli.NewMockUi()
	tr := trellis.NewTrellis()

	infoCommand := NewInfoCommand(ui, tr)
	code := infoCommand.Run([]string{"--json"})

	if code != 0 {
		t.Errorf("expected code 0, got %d\nError: %s", code, ui.ErrorWriter.String())
	}

	output := ui.OutputWriter.String()

	var data infoData
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		t.Fatalf("expected valid JSON output, got error: %s\nOutput: %s", err, output)
	}

	if data.Path == "" {
		t.Error("expected path to be set")
	}

	if len(data.Sites) == 0 {
		t.Error("expected at least one environment in sites")
	}
}
