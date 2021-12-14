package cmd

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/mitchellh/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/trellis"
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

	c.Trellis.CheckVirtualenv(c.UI)

	if err := c.flags.Parse(args); err != nil {
		return 1
	}

	args = c.flags.Args()

	commandArgumentValidator := &CommandArgumentValidator{required: 0, optional: 1}
	commandArgumentErr := commandArgumentValidator.validate(args)
	if commandArgumentErr != nil {
		c.UI.Error(commandArgumentErr.Error())
		c.UI.Output(c.Help())
		return 1
	}

	var files []string
	var environment string

	if len(args) == 1 {
		environment = args[0]
	}

	if environment != "" {
		environmentErr := c.Trellis.ValidateEnvironment(environment)
		if environmentErr != nil {
			c.UI.Error(environmentErr.Error())
			return 1
		}

		if len(c.files) > 0 {
			c.UI.Error("Error: the files option can't be used together with the ENVIRONMENT argument\n")
			c.UI.Output(c.Help())
			return 1
		}

		files = []string{"group_vars/all/vault.yml", fmt.Sprintf("group_vars/%s/vault.yml", environment)}
	} else {
		if len(c.files) == 0 {
			matches, err := filepath.Glob("group_vars/*/vault.yml")

			if err != nil {
				c.UI.Error(err.Error())
				return 1
			}

			files = matches
		} else {
			files = strings.Split(c.files, ",")
		}
	}

	var filesToEncrypt []string

	for _, file := range files {
		isEncrypted, err := trellis.IsFileEncrypted(file)

		if err != nil {
			c.UI.Error(err.Error())
			return 1
		}

		if !isEncrypted {
			filesToEncrypt = append(filesToEncrypt, file)
		}
	}

	if len(filesToEncrypt) == 0 {
		c.UI.Info(color.GreenString("All files already encrypted"))
		return 0
	}

	vaultArgs := []string{"encrypt"}
	vaultArgs = append(vaultArgs, filesToEncrypt...)

	vaultEncrypt := execCommandWithOutput("ansible-vault", vaultArgs, c.UI)
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
Usage: trellis vault encrypt [options] [ENVIRONMENT]

Encrypts files with Ansible Vault.
This command is idempotent and won't try to encrypt already encrypted Vault files.

Trellis docs: https://roots.io/trellis/docs/vault/
Ansible Vault docs: https://docs.ansible.com/ansible/latest/user_guide/vault.html

Encrypt all vault files:

  $ trellis vault encrypt

Encrypt production vault files:

  $ trellis vault encrypt production

Note: when using the ENVIRONMENT argument, 'group_vars/all/vault.yml' is also included.

Encrypt specified files only (multiple files are comma separated):

  $ trellis vault encrypt --files=group_vars/production/vault.yml
  $ trellis vault encrypt --files=group_vars/aaa/vault.yml,group_vars/bbb/vault.yml

Arguments:
  [ENVIRONMENT] Name of environment (ie: production)

Options:
      --files  (multiple) Files to encrypt
               (default: group_vars/all/vault.yml,group_vars/<ENVIRONMENT>/vault.yml)
  -h, --help   show this help
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
