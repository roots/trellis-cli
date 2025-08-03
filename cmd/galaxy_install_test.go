package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestGalaxyInstallRunValidations(t *testing.T) {
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
			galaxyInstallCommand := GalaxyInstallCommand{ui, trellis}

			code := galaxyInstallCommand.Run(tc.args)

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

func TestGalaxyInstallRun(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	cases := []struct {
		name      string
		args      []string
		roleFiles []string
		out       string
		code      int
	}{
		{
			"default",
			[]string{},
			[]string{"galaxy.yml"},
			"ansible-galaxy install -r galaxy.yml",
			0,
		},
		{
			"default",
			[]string{},
			[]string{"requirements.yml"},
			"ansible-galaxy install -r requirements.yml",
			0,
		},
		{
			"default",
			[]string{},
			[]string{},
			"Error: no role file found",
			1,
		},
		{
			"default",
			[]string{},
			[]string{"galaxy.yml", "requirements.yml"},
			"ansible-galaxy install -r galaxy.yml\nWarning: multiple role files found. Defaulting to galaxy.yml",
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			trellis := trellis.NewTrellis()
			galaxyInstallCommand := GalaxyInstallCommand{ui, trellis}

			for _, file := range tc.roleFiles {
				os.Create(file)
			}

			code := galaxyInstallCommand.Run(tc.args)

			if code != tc.code {
				t.Errorf("expected code %d to be %d", code, tc.code)
			}

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}

			for _, file := range tc.roleFiles {
				os.Remove(file)
			}
		})
	}
}
