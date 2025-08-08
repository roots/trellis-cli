package cli_config

import (
	_ "fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFile(t *testing.T) {
	conf := Config{
		AskVaultPass: false,
		LoadPlugins:  true,
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "cli.yml")
	content := `
ask_vault_pass: true
open:
  roots: https://roots.io
`

	if err := os.WriteFile(path, []byte(content), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	if err := conf.LoadFile(path); err != nil {
		t.Fatal(err)
	}

	if conf.LoadPlugins != true {
		t.Errorf("expected LoadPlugins to be true (default value)")
	}

	if conf.AskVaultPass != true {
		t.Errorf("expected AskVaultPass to be true")
	}

	open := conf.Open["roots"]
	expected := "https://roots.io"

	if open != expected {
		t.Errorf("expected open to be %s, got %s", expected, open)
	}
}

func TestLoadEnv(t *testing.T) {
	t.Setenv("TRELLIS_ASK_VAULT_PASS", "true")
	t.Setenv("TRELLIS_NOPE", "foo")
	t.Setenv("ASK_VAULT_PASS", "false")

	conf := Config{
		AskVaultPass: false,
	}

	if err := conf.LoadEnv("TRELLIS_"); err != nil {
		t.Fatal(err)
	}

	if conf.AskVaultPass != true {
		t.Errorf("expected AskVaultPass to be true")
	}
}

func TestLoadBoolParseError(t *testing.T) {
	t.Setenv("TRELLIS_ASK_VAULT_PASS", "foo")

	conf := Config{}

	err := conf.LoadEnv("TRELLIS_")

	if err == nil {
		t.Errorf("expected LoadEnv to return an error")
	}

	msg := err.Error()

	expected := `
Invalid env var config setting: failed to parse value 'TRELLIS_ASK_VAULT_PASS=foo'
'foo' can't be parsed as a boolean
`

	if msg != strings.TrimSpace(expected) {
		t.Errorf("expected error %s got %s", expected, msg)
	}
}

func TestLoadEnvUnsupportedType(t *testing.T) {
	t.Setenv("TRELLIS_OPEN", "foo")

	conf := Config{}

	err := conf.LoadEnv("TRELLIS_")

	if err == nil {
		t.Errorf("expected LoadEnv to return an error")
	}

	msg := err.Error()

	expected := `
Invalid env var config setting: value is an unsupported type.
TRELLIS_OPEN=foo setting of type map[string]string is unsupported.
`

	if msg != strings.TrimSpace(expected) {
		t.Errorf("expected error %s got %s", expected, msg)
	}
}
