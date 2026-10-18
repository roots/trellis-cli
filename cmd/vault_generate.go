package cmd

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/hashicorp/cli"
	"github.com/posener/complete"
	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
	"github.com/roots/trellis-cli/trellis/yamledit"
)

type VaultGenerateCommand struct {
	UI        cli.Ui
	Trellis   *trellis.Trellis
	flags     *flag.FlagSet
	force     bool
	saltsOnly bool
}

func NewVaultGenerateCommand(ui cli.Ui, trellis *trellis.Trellis) *VaultGenerateCommand {
	c := &VaultGenerateCommand{UI: ui, Trellis: trellis}
	c.init()
	return c
}

func (c *VaultGenerateCommand) init() {
	c.flags = flag.NewFlagSet("", flag.ContinueOnError)
	c.flags.Usage = func() { c.UI.Info(c.Help()) }
	c.flags.BoolVar(&c.force, "force", false, "Regenerate secrets that are already set")
	c.flags.BoolVar(&c.saltsOnly, "salts-only", false, "Only generate WordPress keys and salts")
}

func (c *VaultGenerateCommand) Run(args []string) int {
	if err := c.Trellis.LoadProject(); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

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

	environments := c.Trellis.EnvironmentNames()

	if len(args) == 1 {
		if err := c.Trellis.ValidateEnvironment(args[0]); err != nil {
			c.UI.Error(err.Error())
			return 1
		}

		environments = []string{args[0]}
	} else if c.force {
		c.UI.Error("Error: the --force option requires an ENVIRONMENT argument\n")
		c.UI.Output(c.Help())
		return 1
	}

	opts := trellis.VaultGenerateOptions{Force: c.force, SaltsOnly: c.saltsOnly}

	for _, environment := range environments {
		if err := c.generate(environment, opts); err != nil {
			c.UI.Error(fmt.Sprintf("Error generating secrets for %s: %s", environment, err))
			return 1
		}
	}

	return 0
}

func (c *VaultGenerateCommand) generate(environment string, opts trellis.VaultGenerateOptions) error {
	path := filepath.Join("group_vars", environment, "vault.yml")

	data, encrypted, err := c.readVault(path)
	if err != nil {
		return err
	}

	doc, err := yamledit.Parse(data)
	if err != nil {
		return err
	}

	randomString := trellis.RandomStringGenerator{Length: 64}
	sites := c.Trellis.SiteNamesFromEnvironment(environment)

	generated, err := trellis.GenerateVaultSecrets(doc, environment, sites, &randomString, opts)
	if err != nil {
		return err
	}

	if len(generated) == 0 {
		c.UI.Info(fmt.Sprintf("No secrets to generate in %s", path))
		return nil
	}

	if encrypted {
		data, err = doc.Bytes()
		if err == nil {
			err = c.encryptVault(path, data)
		}
	} else {
		err = doc.Save(path)
	}

	if err != nil {
		return err
	}

	c.UI.Info(color.GreenString(fmt.Sprintf("Generated %d secrets in %s", len(generated), path)))

	return nil
}

// readVault returns the plaintext contents of a vault file, decrypting it
// with ansible-vault if needed. A missing file is treated as empty.
func (c *VaultGenerateCommand) readVault(path string) (data []byte, encrypted bool, err error) {
	encrypted, err = trellis.IsFileEncrypted(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	if !encrypted {
		data, err = os.ReadFile(path)
		return data, false, err
	}

	c.Trellis.CheckVirtualenv(c.UI)

	var stdout, stderr bytes.Buffer
	decrypt := command.Cmd("ansible-vault", []string{"decrypt", "--output", "-", path})
	decrypt.Stdout = &stdout
	decrypt.Stderr = &stderr

	if err := decrypt.Run(); err != nil {
		return nil, true, fmt.Errorf("could not decrypt %s: %s", path, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), true, nil
}

// encryptVault encrypts data into path by piping it to ansible-vault so the
// plaintext never touches disk.
func (c *VaultGenerateCommand) encryptVault(path string, data []byte) error {
	var stderr bytes.Buffer
	encrypt := command.Cmd("ansible-vault", []string{"encrypt", "--output", path})
	encrypt.Stdin = bytes.NewReader(data)
	encrypt.Stdout = &stderr
	encrypt.Stderr = &stderr

	if err := encrypt.Run(); err != nil {
		return fmt.Errorf("could not encrypt %s: %s", path, strings.TrimSpace(stderr.String()))
	}

	return nil
}

func (c *VaultGenerateCommand) Synopsis() string {
	return "Generates passwords, keys, and salts in vault files"
}

func (c *VaultGenerateCommand) Help() string {
	helpText := `
Usage: trellis vault generate [options] [ENVIRONMENT]

Generates secrets (passwords and WordPress keys and salts) in group_vars/<environment>/vault.yml files.

Only secrets that are missing, empty, or placeholders (such as "generateme" or the example
passwords in Trellis' default vault files) are generated, so it's safe to run multiple times.
Templated values (containing "{{") are never replaced.

Encrypted vault files are decrypted, updated, and re-encrypted with your project's vault password.

Trellis docs: https://roots.io/trellis/docs/vault/

Generate missing secrets for all environments:

  $ trellis vault generate

Generate missing secrets for production:

  $ trellis vault generate production

Rotate WordPress keys and salts for production (logs out all users):

  $ trellis vault generate --force --salts-only production

Arguments:
  [ENVIRONMENT] Name of environment (ie: production)

Options:
      --force       Regenerate all secrets, including ones already set. Requires ENVIRONMENT.
                    Changed database or user passwords take effect after re-provisioning the server.
      --salts-only  Only generate WordPress keys and salts
  -h, --help        Show this help
`

	return strings.TrimSpace(helpText)
}

func (c *VaultGenerateCommand) AutocompleteArgs() complete.Predictor {
	return c.Trellis.AutocompleteEnvironment(c.flags)
}

func (c *VaultGenerateCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"--force":      complete.PredictNothing,
		"--salts-only": complete.PredictNothing,
	}
}
