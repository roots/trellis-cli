package cmd

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/flags"
	"github.com/roots/trellis-cli/trellis"
)

type VaultEditCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	files   flags.StringSliceVar
}

func NewVaultEditCommand(ui cli.Ui, trellis *trellis.Trellis) *VaultEditCommand {
	c := &VaultEditCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VaultEditCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.Var(&c.files, "f", "File to edit. To edit multiple files, use this option multiple times.")
	c.flags.Var(&c.files, "file", "File to edit. To edit multiple files, use this option multiple times.")
}

func (c *VaultEditCommand) Run(args []string) int {
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

	if c.files == nil {
		matches, err := filepath.Glob("group_vars/*/vault.yml")

		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}

		prompt := promptui.Select{
			Label: "Select a vault file to edit",
			Items: matches,
		}

		_, file, err := prompt.Run()

		if err != nil {
			c.UI.Error("Aborting: no file selected")
			return 1
		}

		c.files = []string{file}
	}

	vaultArgs := []string{"edit"}
	vaultArgs = append(vaultArgs, c.files...)

	vaultEdit := command.WithOptions(command.WithTermOutput(), command.WithLogging(c.UI)).Cmd("ansible-vault", vaultArgs)
	err := vaultEdit.Run()

	if err != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-vault: %s", err))
		return 1
	}

	return 0
}

func (c *VaultEditCommand) Synopsis() string {
	return "Edit an encrypted file in place"
}

func (c *VaultEditCommand) Help() string {
	helpText := `
Usage: trellis vault edit [options]

Edit an encrypted file in place

Trellis docs: https://roots.io/trellis/docs/vault/ 
Ansible Vault docs: https://docs.ansible.com/ansible/latest/user_guide/vault.html

Edit production file:

  $ trellis vault edit -f group_vars/production/vault.yml
  $ trellis vault edit -f group_vars/aaa/vault.yml -f group_vars/bbb/vault.yml

Options:
  -f, --file  File to edit. To edit multiple files, use this option multiple times.
  -h, --help  Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VaultEditCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *VaultEditCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-f":     complete.PredictFiles("*"),
		"--file": complete.PredictFiles("*"),
	}
}
