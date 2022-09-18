package trellis

import (
	"os"
	"testing"
)

func TestCliConfigInit(t *testing.T) {
	config := CliConfig{}
	config.Init()

	_, ok := os.LookupEnv("ANSIBLE_ASK_VAULT_PASS")

	if ok {
		t.Error("expected ANSIBLE_ASK_VAULT_PASS to not be set")
	}
}

func TestCliConfigInitEnablesAskVaultPass(t *testing.T) {
	config := CliConfig{AskVaultPass: true}
	config.Init()

	env := os.Getenv("ANSIBLE_ASK_VAULT_PASS")

	os.Setenv("ANSIBLE_ASK_VAULT_PASS", "")

	if env != "true" {
		t.Error("expected ANSIBLE_ASK_VAULT_PASS to be true")
	}
}
