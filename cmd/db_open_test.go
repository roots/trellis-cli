package cmd

import (
	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
	"os"
	"strings"
	"testing"
)

func TestDBOpenArgumentValidations(t *testing.T) {
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
			[]string{"foo", "bar", "baz"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			mockProject := &MockProject{tc.projectDetected}
			trellis := trellis.NewTrellis(mockProject)

			dbOpenCommand := &DBOpenCommand{UI: ui, Trellis: trellis, dbOpenerFactory: &DBOpenerFactory{}, playbook: &MockPlaybook{ui: ui}}
			dbOpenCommand.init()

			code := dbOpenCommand.Run(tc.args)

			if code != tc.code {
				t.Errorf("%s: expected code %d to be %d", tc.name, code, tc.code)
			}

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}
		})
	}
}

func TestDBOpenAppFlagValidations(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	ui := cli.NewMockUi()
	project := &trellis.Project{}
	trellis := trellis.NewTrellis(project)

	dbOpenCommand := &DBOpenCommand{UI: ui, Trellis: trellis, dbOpenerFactory: &DBOpenerFactory{}, playbook: &MockPlaybook{ui: ui}}
	dbOpenCommand.init()
	dbOpenCommand.app = "unexpected-app"

	code := dbOpenCommand.Run([]string{"production"})

	if code != 1 {
		t.Errorf("expected code %d to be 1", code)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
	expectedOut := "Error initializing new db opener object"
	if !strings.Contains(combined, expectedOut) {
		t.Errorf("expected output %q to contain %q", combined, expectedOut)
	}
}

func TestDBOpenPlaybook(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	ui := cli.NewMockUi()
	project := &trellis.Project{}
	trellis := trellis.NewTrellis(project)
	mockPlaybook := &MockPlaybook{ui: ui}
	dbOpenerFactory := &DBOpenerFactory{}

	dbOpenCommand := &DBOpenCommand{UI: ui, Trellis: trellis, dbOpenerFactory: dbOpenerFactory, playbook: mockPlaybook}
	dbOpenCommand.init()
	dbOpenCommand.app = dbOpenerFactory.GetSupportedApps()[0]

	dbOpenCommand.Run([]string{"production", "example.com"})

	commands := mockPlaybook.GetCommands()
	count := len(commands)
	if count != 1 {
		t.Errorf("expected playbook to be ran exactly once but being ran %d time(s)", count)
	}

	command := commands[0]
	cases := []struct {
		name string
		out  string
	}{
		{
			"correct_playbook",
			"ansible-playbook dump_db_credentials.yml",
		},
		{
			"correct_environment",
			"-e env=production",
		},
		{
			"correct_site",
			"-e site=example.com",
		},
		{
			"correct_destination",
			"-e dest=" + os.TempDir(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(command, tc.out) {
				t.Errorf("%s expected command %s to contain %s", tc.name, command, tc.out)
			}
		})
	}
}
