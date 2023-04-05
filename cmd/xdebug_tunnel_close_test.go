package cmd

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestXdebugTunnelCloseRunValidations(t *testing.T) {
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
			[]string{"1.2.3.4", "foo"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			tunnelCloseCommand := NewXdebugTunnelCloseCommand(ui, trellis)

			code := tunnelCloseCommand.Run(tc.args)

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

func TestXdebugTunnelCloseRun(t *testing.T) {
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
			[]string{"1.2.3.4"},
			"ansible-playbook xdebug-tunnel.yml -e xdebug_remote_enable=0 -e xdebug_tunnel_inventory_host=1.2.3.4",
			0,
		},
		{
			"with_verbose",
			[]string{"--verbose", "1.2.3.4"},
			"ansible-playbook xdebug-tunnel.yml -vvvv -e xdebug_remote_enable=0 -e xdebug_tunnel_inventory_host=1.2.3.4",
			0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			defer MockUiExec(t, ui)()

			tunnelCloseCommand := NewXdebugTunnelCloseCommand(ui, trellis)

			code := tunnelCloseCommand.Run(tc.args)

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
