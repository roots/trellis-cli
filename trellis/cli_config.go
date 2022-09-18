package trellis

import (
	"os"
)

type CliConfig struct {
	AskVaultPass bool              `yaml:"ask_vault_pass"`
	Open         map[string]string `yaml:"open"`
}

func (c *CliConfig) Init() error {
	if c.AskVaultPass {
		// https://docs.ansible.com/ansible/latest/reference_appendices/config.html#default-ask-vault-pass
		os.Setenv("ANSIBLE_ASK_VAULT_PASS", "true")
	}

	return nil
}
