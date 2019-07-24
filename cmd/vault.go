package cmd

import (
	"bufio"
	"os"
	"strings"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

type VaultCommand struct {
	UI      cli.Ui
	Trellis *trellis.Trellis
}

func (c *VaultCommand) Run(args []string) int {
	return cli.RunResultHelp
}

func (c *VaultCommand) Synopsis() string {
	return "Commands for Ansible Vault"
}

func (c *VaultCommand) Help() string {
	helpText := `
Usage: trellis vault <subcommand> [<args>]
`

	return strings.TrimSpace(helpText)
}

func isFileEncrypted(filepath string) (isEncrypted bool, err error) {
	file, err := os.Open(filepath)
	if err != nil {
		return false, err
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	line := scanner.Text()

	if strings.HasPrefix(line, "$ANSIBLE_VAULT") {
		return true, nil
	}

	if err := scanner.Err(); err != nil {
		return false, err
	}

	return false, nil
}
