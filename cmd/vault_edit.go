package cmd

import (
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type VaultEditCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *VaultEditCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	commandArgumentValidator := &CommandArgumentValidator{required: 1, optional: 0}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	file := args[0]

	vaultEdit := execCommand("ansible-vault", []string{"edit", file}, c.UI)
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
Usage: trellis vault edit [options] FILE

Edit an encrypted file in place

Trellis docs: https://roots.io/trellis/docs/vault/
Ansible Vault docs: https://docs.ansible.com/ansible/latest/user_guide/vault.html

Edit production file:

  $ trellis vault edit group_vars/production/vault.yml

Arguments:
  FILE file name to edit

Options:
  -h, --help  show this help
`

	return strings.TrimSpace(helpText)
}
