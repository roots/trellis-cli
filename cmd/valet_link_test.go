package cmd

import (
	"strings"
	"testing"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestValetLinkArgumentValidations(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)

			valetLinkCommand := ValetLinkCommand{ui, trellis}

			code := valetLinkCommand.Run(tc.args)

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

func TestValetLinkValidEnvironmentArgument(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	cases := []struct {
		name            string
		projectDetected bool
		args            []string
		out             string
	}{
		{
			"default_environment",
			true,
			nil,
			"Linking environment development...",
		},
		{
			"custom_environment",
			true,
			[]string{"production"},
			"Linking environment production...",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)

			valetLinkCommand := ValetLinkCommand{ui, trellis}

			valetLinkCommand.Run(tc.args)

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}
		})
	}
}

func TestValetLinkInvalidEnvironmentArgument(t *testing.T) {
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
		t.Run(tc.name, func(t *testing.T) {
			trellis := trellis.NewMockTrellis(tc.projectDetected)

			valetLinkCommand := ValetLinkCommand{ui, trellis}

			code := valetLinkCommand.Run(tc.args)

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

func TestValetLinkRun(t *testing.T) {
	trellisProject := trellis.NewTrellis()

	defer trellis.TestChdir(t, "../trellis/testdata/trellis")()

	if err := trellisProject.LoadProject(); err != nil {
		t.Fatal(err)
	}

	trellis := trellis.NewMockTrellis(true)

	cases := []struct {
		name string
		out  string
	}{
		{
			"insecure_site",
			"valet link example",
		},
		{
			"secure_site",
			"valet link --secure secure",
		},
		{
			"subdomain_site",
			"valet link sub.domain",
		},
		{
			"no_ssl_site",
			"valet link no-ssl",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			valetLinkCommand := ValetLinkCommand{ui, trellis}
			code := valetLinkCommand.Run([]string{"valet-link"})

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}

			if code != 0 {
				t.Errorf("expected code %d to be %d", code, 0)
			}
		})
	}
}
