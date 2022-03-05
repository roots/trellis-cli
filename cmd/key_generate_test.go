package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/command"
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

	tmpDir := t.TempDir()

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

	tmpDir := t.TempDir()

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

	tmpDir := t.TempDir()

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

func TestKeyGenerateKeyscan(t *testing.T) {
	defer trellis.LoadFixtureProject(t)()
	trellis := trellis.NewTrellis()

	tmpDir := t.TempDir()

	// fake gh binary to satisfy ok.LookPath
	ghPath := filepath.Join(tmpDir, "gh")
	os.OpenFile(ghPath, os.O_CREATE, 0555)
	path := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("PATH=%s:%s", path, tmpDir))

	mockExecCommand := func(command string, args []string) *exec.Cmd {
		cs := []string{"-test.run=TestKeyGenerateHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1", fmt.Sprintf("GO_TEST_HELPER_TMP_PATH=%s", tmpDir)}
		return cmd
	}

	command.Mock(mockExecCommand)
	defer command.Restore()

	ui := cli.NewMockUi()
	keyGenerateCommand := NewKeyGenerateCommand(ui, trellis)

	code := keyGenerateCommand.Run([]string{"--path", tmpDir})

	if code != 0 {
		t.Errorf("expected code %d to be %d", code, 0)
	}
}

func TestParseAnsibleHosts(t *testing.T) {
	output := `
  hosts (3):
    192.168.56.5
    192.168.56.10
    your_server_hostname
`

	hosts := parseAnsibleHosts(output)

	expectedHosts := []string{
		"192.168.56.5",
		"192.168.56.10",
	}

	if !reflect.DeepEqual(hosts, expectedHosts) {
		t.Errorf("expected hosts %q to equal %q", hosts, expectedHosts)
	}
}

func TestKeyGenerateHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	switch os.Args[3] {
	case "ansible":
		hosts := `
good_host
bad_host
your_server_hostname
`
		fmt.Fprintf(os.Stdout, hosts)
		os.Exit(0)
	case "ssh-keyscan":
		switch os.Args[len(os.Args)-1] {
		case "good_host":
			// return fake hash for a good host
			host := "|1|5XBUprxMy6abCgLQkQ0= ssh-ed25519 AAAAC3NzaC1lZYqEOf"
			fmt.Fprintf(os.Stdout, host)
			os.Exit(0)
		case "bad_host":
			// simulate error for a bad host
			os.Exit(1)
		}
	case "ssh-keygen":
		tmpDir := os.Getenv("GO_TEST_HELPER_TMP_PATH")
		path := filepath.Join(tmpDir, "trellis_example_com_ed25519.pub")
		os.OpenFile(path, os.O_CREATE, 0644)
		path = filepath.Join(tmpDir, "trellis_example_com_ed25519")
		os.OpenFile(path, os.O_CREATE, 0644)
		os.Exit(0)
	case "gh":
		// make all gh commands succeed. No output needed
		os.Exit(0)
	default:
		t.Fatalf("unexpected command %s", os.Args[3])
	}
}
