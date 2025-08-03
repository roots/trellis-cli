package cmd

import (
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestDeployRunValidations(t *testing.T) {
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
			"Usage: trellis",
			1,
		},
		{
			"invalid_env",
			true,
			[]string{"foo"},
			"Error: foo is not a valid environment",
			1,
		},
		{
			"invalid_env_with_site",
			true,
			[]string{"foo", "example.com"},
			"Error: foo is not a valid environment",
			1,
		},
		{
			"invalid_site",
			true,
			[]string{"production", "nosite"},
			"Error: nosite is not a valid site",
			1,
		},
		{
			"too_many_args",
			true,
			[]string{"production", "site", "foo"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			deployCommand := NewDeployCommand(ui, trellis)

			code := deployCommand.Run(tc.args)

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

func TestDeployRun(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()

	cases := []struct {
		name string
		args []string
		out  string
		code int
	}{
		{
			"default",
			[]string{"production", "example.com"},
			"ansible-playbook deploy.yml -e env=production -e site=example.com",
			0,
		},
		{
			"site_not_needed_in_default_case",
			[]string{"production"},
			"ansible-playbook deploy.yml -e env=production -e site=example.com",
			0,
		},
		{
			"with_extra_vars",
			[]string{"-extra-vars", "k=v foo=bar", "production"},
			"ansible-playbook deploy.yml -e k=v foo=bar -e env=production -e site=example.com",
			0,
		},
		{
			"with_branch",
			[]string{"-branch", "feature-123", "production"},
			"ansible-playbook deploy.yml -e branch=feature-123 -e env=production -e site=example.com",
			0,
		},
		{
			"with_verbose",
			[]string{"--verbose", "production"},
			"ansible-playbook deploy.yml -vvvv -e env=production -e site=example.com",
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			deployCommand := NewDeployCommand(ui, trellis)
			code := deployCommand.Run(tc.args)

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

func TestDeployNotAllowedForDevelopment(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	ui := cli.NewMockUi()
	trellis := trellis.NewMockTrellis(true)
	deployCommand := NewDeployCommand(ui, trellis)

	code := deployCommand.Run([]string{"development"})

	if code != 1 {
		t.Errorf("expected code %d to be 1", code)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

	expected := "allow_development_deploys"

	if !strings.Contains(combined, expected) {
		t.Errorf("expected output %q to contain %q", combined, expected)
	}
}

func TestDeployAllowedForDevelopmentWhenEnabled(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	ui := cli.NewMockUi()
	trellis := trellis.NewMockTrellis(true)
	trellis.CliConfig.AllowDevelopmentDeploys = true
	deployCommand := NewDeployCommand(ui, trellis)

	deployCommand.Run([]string{"development"})

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

	expected := "allow_development_deploys"

	if strings.Contains(combined, expected) {
		t.Errorf("expected output %q to NOT contain %q", combined, expected)
	}
}
