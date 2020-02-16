package cmd

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

func TestSSLFetchArgumentValidations(t *testing.T) {
	ui := cli.NewMockUi()

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
			[]string{"foo", "bar"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		mockProject := &MockProject{tc.projectDetected}
		trellis := trellis.NewTrellis(mockProject)

		sslFetchCommand := SSLFetchCommand{UI: ui, Trellis: trellis, playbook: &MockPlaybook{ui: ui}}

		code := sslFetchCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestSSLFetchInvalidEnvironmentArgument(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	ui := cli.NewMockUi()

	cases := []struct {
		name            string
		projectDetected bool
		args            []string
		out             string
		code            int
	}{
		{
			"invalid_env",
			true,
			[]string{"foo"},
			"Error: foo is not a valid environment",
			1,
		},
	}

	for _, tc := range cases {
		mockProject := &MockProject{tc.projectDetected}
		trellis := trellis.NewTrellis(mockProject)

		sslFetchCommand := SSLFetchCommand{UI: ui, Trellis: trellis, playbook: &MockPlaybook{ui: ui}}

		code := sslFetchCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestSSLFetchRun(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	ui := cli.NewMockUi()
	project := &trellis.Project{}
	trellis := trellis.NewTrellis(project)
	mockPlaybook := &MockPlaybook{ui: ui}
	sslFetchCommand := &SSLFetchCommand{UI: ui, Trellis: trellis, playbook: mockPlaybook}

	cases := []struct {
		args    []string
		commandCount int
		command string
		out     string
		code    int
	}{
		{
			[]string{"production"},
			1,
			"ansible-playbook ssl-fetch.yml -e env=production -e dest=",
			"SSL certificates fetched into",
			0,
		},
	}

	for _, tc := range cases {
		code := sslFetchCommand.Run(tc.args)

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
		commands := mockPlaybook.GetCommands()

		commandCount := len(commands)
		if commandCount != tc.commandCount {
			t.Errorf("expected playbook to be ran excatly %d time(s), but ran %d time(s)", tc.commandCount, commandCount)
		}

		command := commands[0]
		if !strings.Contains(command, tc.command) {
			t.Errorf("Expected command %s to contain %s", command, tc.command)
		}

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}

		if code != tc.code {
			t.Errorf("expected code %d to be %d", code, tc.code)
		}
	}
}
