package cmd

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/pkg/flags"
	"github.com/roots/trellis-cli/trellis"
)

type VaultEncryptCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	files   flags.StringSliceVar
}

func NewVaultEncryptCommand(ui cli.Ui, trellis *trellis.Trellis) *VaultEncryptCommand {
	c := &VaultEncryptCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VaultEncryptCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.Var(&c.files, "f", "File to encrypt. To encrypt multiple files, use this option multiple times.")
	c.flags.Var(&c.files, "file", "File to encrypt. To encrypt multiple files, use this option multiple times.")
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

	var environment string

	if len(args) == 1 {
		environment = args[0]

		environmentErr := c.Trellis.ValidateEnvironment(environment)
		if environmentErr != nil {
			c.UI.Error(environmentErr.Error())
			return 1
		}

		if len(c.files) > 0 {
			c.UI.Error("Error: the file option can't be used together with the ENVIRONMENT argument\n")
			c.UI.Output(c.Help())
			return 1
		}
	}

	if environment == "" {
		if len(c.files) == 0 {
			matches, err := filepath.Glob("group_vars/*/vault.yml")

			if err != nil {
				c.UI.Error(err.Error())
				return 1
			}

			c.files = matches
		}
	} else {
		c.files = []string{"group_vars/all/vault.yml", fmt.Sprintf("group_vars/%s/vault.yml", environment)}
	}

	var filesToEncrypt []string

	for _, file := range c.files {
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

	mockUi := cli.NewMockUi()
	vaultEncrypt := command.WithOptions(
		command.WithUiOutput(mockUi),
		command.WithLogging(c.UI),
	).Cmd("ansible-vault", vaultArgs)

	if err := vaultEncrypt.Run(); err != nil {
		c.UI.Error(mockUi.ErrorWriter.String())
		return 1
	}

	c.UI.Info(color.GreenString("Encryption successful"))

	return 0
}

func (c *VaultEncryptCommand) Synopsis() string {
	return "Encrypts files with Ansible Vault"
}

func (c *VaultEncryptCommand) Help() string {
	helpText := `
Usage: trellis vault encrypt [options] [ENVIRONMENT]

Encrypts files with Ansible Vault.
This command is idempotent and safe to run on already encrypted files.

Trellis docs: https://roots.io/trellis/docs/vault/ 
Ansible Vault docs: https://docs.ansible.com/ansible/latest/user_guide/vault.html

Encrypt all vault files:

  $ trellis vault encrypt

Encrypt production vault files:

  $ trellis vault encrypt production

Note: when using the ENVIRONMENT argument, 'group_vars/all/vault.yml' is also included.

Encrypt specified files:

  $ trellis vault encrypt -f group_vars/production/vault.yml
  $ trellis vault encrypt -f group_vars/aaa/vault.yml -f group_vars/bbb/vault.yml

Arguments:
  [ENVIRONMENT] Name of environment (ie: production)

Options:
  -f, --file  File to encrypt. To encrypt multiple files, use this option multiple times.
  -h, --help  Show this help
`

	return CreateHelp("vault encrypt", c.Synopsis(), strings.TrimSpace(helpText))
}

func (c *VaultEncryptCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment(c.flags)
}

func (c *VaultEncryptCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-f":     complete.PredictFiles("*"),
		"--file": complete.PredictFiles("*"),
	}
}
