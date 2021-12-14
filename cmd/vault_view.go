package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/trellis"
)

type VaultViewCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	files   string
}

func NewVaultViewCommand(ui cli.Ui, trellis *trellis.Trellis) *VaultViewCommand {
	c := &VaultViewCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VaultViewCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.files, "files", "", "Files to view. Must be comma separated without spaces in between.")
}

func (c *VaultViewCommand) Run(args []string) int {
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

	environment := args[0]

	environmentErr := c.Trellis.ValidateEnvironment(environment)
	if environmentErr != nil {
		c.UI.Error(environmentErr.Error())
		return 1
	}

	var files []string

	vaultArgs := []string{"view"}

	if len(c.files) == 0 {
		files = []string{"group_vars/all/vault.yml", fmt.Sprintf("group_vars/%s/vault.yml", environment)}
	} else {
		files = strings.Split(c.files, ",")
	}

	vaultArgs = append(vaultArgs, files...)
	vaultView := execCommandWithOutput("ansible-vault", vaultArgs, c.UI)
	_ = vaultView.Run()

	return 0
}

func (c *VaultViewCommand) Synopsis() string {
	return "Open, decrypt and view existing vaulted files"
}

func (c *VaultViewCommand) Help() string {
	helpText := `
Usage: trellis vault encrypt [options] ENVIRONMENT

Open, decrypt and view existing vaulted files

Trellis docs: https://roots.io/trellis/docs/vault/
Ansible Vault docs: https://docs.ansible.com/ansible/latest/user_guide/vault.html

View production vault files:

  $ trellis vault view production

View specified files for production environment:

  $ trellis vault view --files=group_vars/production/vault.yml production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
      --files  (multiple) Files to view
               (default: group_vars/all/vault.yml group_vars/<ENVIRONMENT>/vault.yml)
  -h, --help   show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VaultViewCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment()
}

func (c *VaultViewCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--files": complete.PredictNothing,
	}
}
