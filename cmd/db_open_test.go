package cmd

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/db_opener"
	"github.com/roots/trellis-cli/trellis"
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
			trellis := trellis.NewMockTrellis(tc.projectDetected)

			dbOpenCommand := &DBOpenCommand{UI: ui, Trellis: trellis, dbOpenerFactory: &db_opener.Factory{}, playbook: &AdHocPlaybook{}}
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
	trellis := trellis.NewTrellis()

	dbOpenCommand := &DBOpenCommand{UI: ui, Trellis: trellis, dbOpenerFactory: &db_opener.Factory{}, playbook: &AdHocPlaybook{}}
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
	trellis := trellis.NewTrellis()

	dbOpenerFactory := &db_opener.Factory{}

	cases := []struct {
		name string
		args []string
		out  *regexp.Regexp
	}{
		{
			"default",
			[]string{"-app=" + dbOpenerFactory.GetSupportedApps()[0], "production", "example.com"},
			regexp.MustCompile("ansible-playbook dump_db_credentials.yml -e dest=" + os.TempDir() + ".*.json -e env=production -e site=example.com"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			dbOpenCommand := NewDBOpenCommand(ui, trellis)
			dbOpenCommand.Run(tc.args)

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !tc.out.MatchString(combined) {
				t.Errorf("expected output %q to match %q", combined, tc.out)
			}
		})
	}
}
