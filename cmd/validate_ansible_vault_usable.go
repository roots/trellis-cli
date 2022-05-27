package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/roots/trellis-cli/command"
	"github.com/roots/trellis-cli/trellis"
)

func validateAnsibleVaultUsable(trellisPath string) (err error) {
	_, err = command.Cmd("which", []string{"ansible-vault"}).CombinedOutput()
	if err != nil {
		return fmt.Errorf("command not found: ansible-vault")
	}

	helpText := "see: https://docs.roots.io/trellis/master/vault/#storing-your-password"

	dummyString := "foo bar"
	err = command.Cmd("ansible-vault", []string{"encrypt_string", dummyString}).Run()
	if err != nil {
		return fmt.Errorf("unable to encrypt_string. probably: vault_password_file not exists. %s", helpText)
	}

	path := findFirstEncryptedFilePath(trellisPath)
	if path == "" {
		// No encrypted files found. Assume ansible-vault is working.
		return nil
	}

	err = command.Cmd("ansible-vault", []string{"decrypt", "--output", "-", path}).Run()
	if err != nil {
		return fmt.Errorf("unable to decrypt vault file %s. probably: incorrect vault pass. %s", path, helpText)
	}

	return nil
}

func findFirstEncryptedFilePath(trellisPath string) string {
	defaultVaults := []string{
		"group_vars/development/vault.yml", // Start with development because it contains less important secrets.
		"group_vars/staging/vault.yml",
		"group_vars/all/vault.yml",
		"group_vars/production/vault.yml",
	}
	for _, vault := range defaultVaults {
		path := filepath.Join(trellisPath, vault)

		isEncrypted, _ := trellis.IsFileEncrypted(path)
		if isEncrypted {
			return path
		}
	}

	result := ""
	filepath.Walk(
		filepath.Join(trellisPath, "group_vars"),
		func(path string, fi os.FileInfo, _ error) error {
			if result != "" {
				return nil
			}

			if fi.IsDir() {
				return nil
			}

			isEncrypted, _ := trellis.IsFileEncrypted(path)
			if isEncrypted {
				result = path
			}

			return nil
		})

	return result
}
