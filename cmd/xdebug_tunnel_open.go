package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/trellis"
)

type XdebugTunnelOpenCommand struct {
	UI       cli.Ui
	Trellis  *trellis.Trellis
	flags    *flag.FlagSet
	playbook PlaybookRunner
	verbose  bool
}

func NewXdebugTunnelOpenCommand(ui cli.Ui, trellis *trellis.Trellis) *XdebugTunnelOpenCommand {
	c := &XdebugTunnelOpenCommand{UI: ui, Trellis: trellis, playbook: &Playbook{ui: ui}}
	c.init()
	return c
}

func (c *XdebugTunnelOpenCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.verbose, "verbose", false, "Enable Ansible's verbose mode")
}

func (c *XdebugTunnelOpenCommand) Run(args []string) int {
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

	c.playbook.SetRoot(c.Trellis.Path)

	host := args[0]
	inventoryHost := fmt.Sprintf("xdebug_tunnel_inventory_host=%s", host)
	playbookArgs := []string{"-e", "xdebug_remote_enable=1", "-e", "sshd_allow_tcp_forwarding=yes", "-e", inventoryHost}

	if c.verbose {
		playbookArgs = append(playbookArgs, "-vvvv")
	}

	if err := c.playbook.Run("xdebug-tunnel.yml", playbookArgs); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	return 0
}

func (c *XdebugTunnelOpenCommand) Synopsis() string {
	return "Opens a remote SSH tunnel to allow remote Xdebug connections."
}

func (c *XdebugTunnelOpenCommand) Help() string {
	helpText := `
Usage: trellis xdebug-tunnel open [options] HOST

Opens a remote SSH tunnel to allow remote Xdebug connections.

Documentation: https://docs.roots.io/trellis/master/debugging-php/#using-xdebug-in-production

Open Xdebug tunnel on host 1.2.3.4:

  $ trellis xdebug-tunnel open 1.2.3.4

Arguments:
  HOST Host (IP or name) to open the xdebug tunnel on

Options:
      --verbose Enable Ansible's verbose mode
  -h, --help    show this help
`

	return strings.TrimSpace(helpText)
}

func (c *XdebugTunnelOpenCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--verbose": complete.PredictNothing,
	}
}
