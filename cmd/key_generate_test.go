package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestKeyGenerateRunValidations(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()

	cases := []struct {
		name            string
		projectDetected bool
		args            []string
		out             string
		code            int
	}{
		{
			"no_project",
			false,
			nil,
			"No Trellis project detected",
			1,
		},
		{
			"too_many_args",
			true,
			[]string{"foo"},
			"Error: too many arguments",
			1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ui := cli.NewMockUi()
			trellis := trellis.NewMockTrellis(tc.projectDetected)
			keyGenerateCommand := NewKeyGenerateCommand(ui, trellis)

			code := keyGenerateCommand.Run(tc.args)

			if code != tc.code {
				t.Errorf("expected code %d to be %d", code, tc.code)
			}

			combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

			if !strings.Contains(combined, tc.out) {
				t.Errorf("expected output %q to contain %q", combined, tc.out)
			}
		})
	}
}

func TestKeyGenerateNewKey(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()

	tmpDir, _ := ioutil.TempDir("", "key_generate_test")
	defer os.RemoveAll(tmpDir)

	ui := cli.NewMockUi()
	keyGenerateCommand := NewKeyGenerateCommand(ui, trellis)

	code := keyGenerateCommand.Run([]string{"--path", tmpDir, "--no-github"})

	if code != 0 {
		t.Errorf("expected code %d to be %d", code, 0)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
	expected := fmt.Sprintf("Generated SSH key [%s]", filepath.Join(tmpDir, "trellis_example_com_ed25519"))

	if !strings.Contains(combined, expected) {
		t.Errorf("expected output %q to contain %q", combined, expected)
	}

	expected = fmt.Sprintf("Moved public key [%s]", filepath.Join(trellis.Path, "public_keys", "trellis_example_com_ed25519.pub"))

	if !strings.Contains(combined, expected) {
		t.Errorf("expected output %q to contain %q", combined, expected)
	}
}

func TestKeyGenerateExistingPrivateKey(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()

	tmpDir, _ := ioutil.TempDir("", "key_generate_test")
	defer os.RemoveAll(tmpDir)

	privateKeyPath := filepath.Join(tmpDir, "trellis_example_com_ed25519")
	ioutil.WriteFile(privateKeyPath, []byte{}, 0666)

	ui := cli.NewMockUi()
	keyGenerateCommand := NewKeyGenerateCommand(ui, trellis)

	code := keyGenerateCommand.Run([]string{"--path", tmpDir, "--no-github"})

	if code != 1 {
		t.Errorf("expected code %d to be %d", code, 1)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
	expected := "keys already exist."

	if !strings.Contains(combined, expected) {
		t.Errorf("expected output %q to contain %q", combined, expected)
	}
}

func TestKeyGenerateExistingPublicKey(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()

	tmpDir, _ := ioutil.TempDir("", "key_generate_test")
	defer os.RemoveAll(tmpDir)

	os.Mkdir(filepath.Join(trellis.Path, "public_keys"), os.ModePerm)
	publicKeyPath := filepath.Join(trellis.Path, "public_keys", "trellis_example_com_ed25519.pub")
	err := ioutil.WriteFile(publicKeyPath, []byte{}, 0666)

	if err != nil {
		t.Fatal(err)
	}

	ui := cli.NewMockUi()
	keyGenerateCommand := NewKeyGenerateCommand(ui, trellis)

	code := keyGenerateCommand.Run([]string{"--path", tmpDir, "--no-github"})

	if code != 1 {
		t.Errorf("expected code %d to be %d", code, 1)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()
	expected := "keys already exist."

	if !strings.Contains(combined, expected) {
		t.Errorf("expected output %q to contain %q", combined, expected)
	}
}
