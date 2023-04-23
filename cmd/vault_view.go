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

type VaultViewCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
	flags   *flag.FlagSet
	files   flags.StringSliceVar
}

func NewVaultViewCommand(ui cli.Ui, trellis *trellis.Trellis) *VaultViewCommand {
	c := &VaultViewCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VaultViewCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.Var(&c.files, "f", "File to view. To view multiple files, use this option multiple times.")
	c.flags.Var(&c.files, "file", "File to view. To view multiple files, use this option multiple times.")
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

	vaultArgs := []string{"view"}

	if environment == "" {
		if len(c.files) == 0 {
			matches, err := filepath.Glob("group_vars/*/vault.yml")

			if err != nil {
				c.UI.Error(err.Error())
				return 1
			}

			prompt := promptui.Select{
				Label: "Select a vault file to view",
				Items: matches,
			}

			_, file, err := prompt.Run()

			if err != nil {
				c.UI.Error("Aborting: no file selected")
				return 1
			}

			c.files = []string{file}
		}
	} else {
		c.files = []string{"group_vars/all/vault.yml", fmt.Sprintf("group_vars/%s/vault.yml", environment)}
	}

	vaultArgs = append(vaultArgs, c.files...)
	vaultView := command.WithOptions(command.WithTermOutput(), command.WithLogging(c.UI)).Cmd("ansible-vault", vaultArgs)
	_ = vaultView.Run()

	return 0
}

func (c *VaultViewCommand) Synopsis() string {
	return "Open, decrypt and view existing vaulted files"
}

func (c *VaultViewCommand) Help() string {
	helpText := `
Usage: trellis vault view [options] [ENVIRONMENT]

Open, decrypt and view existing vaulted files

Trellis docs: https://roots.io/trellis/docs/vault/ 
Ansible Vault docs: https://docs.ansible.com/ansible/latest/user_guide/vault.html

View production vault files:

  $ trellis vault view production

View specified files for production environment:

  $ trellis vault view -f group_vars/production/vault.yml
  $ trellis vault view -f group_vars/aaa/vault.yml -f group_vars/bbb/vault.yml

Arguments:
  ENVIRONMENT Name of environment (ie: production)

Options:
  -f, --file  File to view. To view multiple files, use this option multiple times.
  -h, --help  Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VaultViewCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment(c.flags)
}

func (c *VaultViewCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-f":     complete.PredictFiles("*"),
		"--file": complete.PredictFiles("*"),
	}
}
