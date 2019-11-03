package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type VaultEditCommand struct {
	UI              cli.Ui
	Trellis         *trellis.Trellis
	CommandExecutor CommandExecutor
}

func NewVaultEditCommand(ui cli.Ui, trellis *trellis.Trellis, ce CommandExecutor) *VaultEditCommand {
	c := &VaultEditCommand{UI: ui, Trellis: trellis, CommandExecutor: ce}
	return c
}

func (c *VaultEditCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	argCountErr := validateArgumentCount(args, 1, 0)
	if argCountErr != nil {
		c.UI.Error(argCountErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	file := args[0]

	ansibleVault, lookErr := c.CommandExecutor.LookPath("ansible-vault")
	if lookErr != nil {
		c.UI.Error(fmt.Sprintf("ansible-vault command not found: %s", lookErr))
		return 1
	}

	vaultArgs := []string{"ansible-vault", "edit", file}
	env := os.Environ()
	execErr := c.CommandExecutor.Exec(ansibleVault, vaultArgs, env)

	if execErr != nil {
		c.UI.Error(fmt.Sprintf("Error running ansible-vault: %s", execErr))
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
