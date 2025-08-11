package cmd

import (
	"flag"
	"strings"

	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/ansible"
	"github.com/roots/trellis-cli/trellis"
)

type XdebugTunnelCloseCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	verbose bool
}

func NewXdebugTunnelCloseCommand(ui cli.Ui, trellis *trellis.Trellis) *XdebugTunnelCloseCommand {
	c := &XdebugTunnelCloseCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *XdebugTunnelCloseCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.verbose, "verbose", false, "Enable Ansible's verbose mode")
}

func (c *XdebugTunnelCloseCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	host := args[0]

	playbook := ansible.Playbook{
		Name:    "xdebug-tunnel.yml",
		Verbose: c.verbose,
		ExtraVars: map[string]string{
			"xdebug_tunnel_inventory_host": host,
			"xdebug_remote_enable":         "0",
		},
	}

	xdebugClose := command.WithOptions(command.WithTermOutput(), command.WithLogging(c.UI)).Cmd("ansible-playbook", playbook.CmdArgs())

	if err := xdebugClose.Run(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *XdebugTunnelCloseCommand) Synopsis() string {
	return "Closes the remote Xdebug tunnel connection."
}

func (c *XdebugTunnelCloseCommand) Help() string {
	helpText := `
Usage: trellis xdebug-tunnel close [options] HOST

Closes the remote Xdebug tunnel connection.

Documentation: https://roots.io/trellis/docs/debugging-php/#using-xdebug-in-production

Close Xdebug tunnel on host 1.2.3.4:

  $ trellis xdebug-tunnel close 1.2.3.4

Arguments:
  HOST Host (IP or name) to close the xdebug tunnel on

Options:
      --verbose Enable Ansible's verbose mode
  -h, --help    show this help
`

	return CreateHelp("xdebug-tunnel close", c.Synopsis(), strings.TrimSpace(helpText))
}

func (c *XdebugTunnelCloseCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--verbose": complete.PredictNothing,
	}
}
