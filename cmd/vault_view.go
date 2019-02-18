package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
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

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	var environment string
	var files []string

	switch len(args) {
	case 0:
		c.UI.Error("Error: missing ENVIRONMENT argument\n")
		c.UI.Output(c.Help())
		return 1
	case 1:
		environment = args[0]
	default:
		c.UI.Error(fmt.Sprintf("Error: too many arguments (expected 1, got %d)\n", len(args)))
		c.UI.Output(c.Help())
		return 1
	}

	vaultArgs := []string{"view"}

	if len(c.files) == 0 {
		files = []string{"group_vars/all/vault.yml", fmt.Sprintf("group_vars/%s/vault.yml", environment)}
	} else {
		files = strings.Split(c.files, ",")
	}

	vaultArgs = append(vaultArgs, files...)

	vaultView := execCommand("ansible-vault", vaultArgs...)
	logCmd(vaultView, c.UI, true)
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
