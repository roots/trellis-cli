package cmd

import (
	"flag"
	"strings"
	"os"
	"path/filepath"

	"github.com/manifoldco/promptui"
	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/trellis"
)

type VmDeleteCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	force   bool
}

func NewVmDeleteCommand(ui cli.Ui, trellis *trellis.Trellis) *VmDeleteCommand {
	c := &VmDeleteCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VmDeleteCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.force, "force", false, "Delete VM without confirmation.")
}

func (c *VmDeleteCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	siteName, _, err := c.Trellis.MainSiteFromEnvironment("development")
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	manager, err := newVmManager(c.Trellis, c.UI)
	if err != nil {
		c.UI.Error("Error: " + err.Error())
		return 1
	}

	if c.force || c.confirmDeletion() {
		if err := manager.DeleteInstance(siteName); err != nil {
			c.UI.Error("Error: " + err.Error())
			return 1
			}
		
		// Remove instance file if it exists
		instancePath := filepath.Join(c.Trellis.ConfigPath(), "lima", "instance")
		os.Remove(instancePath) // Ignore errors as file may not exist
	}

	return 0
}

func (c *VmDeleteCommand) Synopsis() string {
	return "Deletes the development virtual machine."
}

func (c *VmDeleteCommand) Help() string {
	helpText := `
Usage: trellis vm delete [options]

Deletes the development virtual machine.
VMs must be in a stopped state before they can be deleted.

Delete without prompting for confirmation:
  $ trellis vm delete --force

Options:
  --force     Delete VM without confirmation.
  -h, --help  Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VmDeleteCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--force": complete.PredictNothing,
	}
}

func (c *VmDeleteCommand) confirmDeletion() bool {
	prompt := promptui.Prompt{
		Label:     "Delete virtual machine",
		IsConfirm: true,
	}

	_, err := prompt.Run()

	if err != nil {
		c.UI.Info("Aborted. Not deleting virtual machine.")
		return false
	}

	return true
}
