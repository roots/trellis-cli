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

type VaultDecryptCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	files   string
}

func NewVaultDecryptCommand(ui cli.Ui, trellis *trellis.Trellis) *VaultDecryptCommand {
	c := &VaultDecryptCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VaultDecryptCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.StringVar(&c.files, "files", "", "Files to decrypt. Must be comma separated without spaces in between.")
}

func (c *VaultDecryptCommand) Run(args []string) int {
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

	vaultArgs := []string{"decrypt"}

	if len(c.files) == 0 {
		files = []string{"group_vars/all/vault.yml", fmt.Sprintf("group_vars/%s/vault.yml", environment)}
	} else {
		files = strings.Split(c.files, ",")
	}

	vaultArgs = append(vaultArgs, files...)

	vaultDecrypt := execCommand("ansible-vault", vaultArgs...)
	logCmd(vaultDecrypt, c.UI, true)
	err := vaultDecrypt.Run()

	if err == nil {
		c.UI.Info(color.GreenString("Decryption successful"))
	}

	return 0
}

func (c *VaultDecryptCommand) Synopsis() string {
	return "Decrypts files with Ansible Vault"
}

func (c *VaultDecryptCommand) Help() string {
	helpText := `
Usage: trellis vault decrypt [options] ENVIRONMENT

Decrypts files with Ansible Vault for the specified environment

Trellis docs: https://roots.io/trellis/docs/vault/
Ansible Vault docs: https://docs.ansible.com/ansible/latest/user_guide/vault.html

Decrypt production vault files:

  $ trellis vault decrypt production

Decrypt specified files for production environment:

  $ trellis vault decrypt --files=group_vars/production/vault.yml production

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
  -h,      --help show this help
  --files, (multiple) Files to decrypt
           (default: group_vars/all/vault.yml group_vars/<ENVIRONMENT>/vault.yml)
`

	return strings.TrimSpace(helpText)
}

func (c *VaultDecryptCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment()
}

func (c *VaultDecryptCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--files": complete.PredictNothing,
	}
}
