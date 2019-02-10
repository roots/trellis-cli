package cmd

import (
	"flag"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"trellis-cli/trellis"
)

type VaultEncryptCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	files   string
}

func NewVaultEncryptCommand(ui cli.Ui, trellis *trellis.Trellis) *VaultEncryptCommand {
	c := &VaultEncryptCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VaultEncryptCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.files, "files", "", "Files to encrypt. Must be comma separated without spaces in between.")
}

func (c *VaultEncryptCommand) Run(args []string) int {
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

	vaultArgs := []string{"encrypt"}

	if len(c.files) == 0 {
		files = []string{"group_vars/all/vault.yml", fmt.Sprintf("group_vars/%s/vault.yml", environment)}
	} else {
		files = strings.Split(c.files, ",")
	}

	vaultArgs = append(vaultArgs, files...)

	vaultEncrypt := execCommand("ansible-vault", vaultArgs...)
	logCmd(vaultEncrypt, c.UI, true)
	err := vaultEncrypt.Run()

	if err == nil {
		c.UI.Info(color.GreenString("Encryption successful"))
	}

	return 0
}

func (c *VaultEncryptCommand) Synopsis() string {
	return "Encrypts files with Ansible Vault"
}

func (c *VaultEncryptCommand) Help() string {
	helpText := `
Usage: trellis vault encrypt [options] ENVIRONMENT

Encrypts files with Ansible Vault for the specified environment

Trellis docs: https://roots.io/trellis/docs/vault/
Ansible Vault docs: https://docs.ansible.com/ansible/latest/user_guide/vault.html

Encrypt production vault files:

  $ trellis vault encrypt production

Encrypt specified files for production environment:

  $ trellis vault encrypt --files=group_vars/production/vault.yml production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
  -h,      --help show this help
  --files, (multiple) Files to encrypt
           (default: group_vars/all/vault.yml group_vars/<ENVIRONMENT>/vault.yml)
`

	return strings.TrimSpace(helpText)
}

func (c *VaultEncryptCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment()
}

func (c *VaultEncryptCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--files": complete.PredictNothing,
	}
}
