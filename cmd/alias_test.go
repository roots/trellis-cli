package cmd

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
	"trellis-cli/trellis"
)

func TestAliasArgumentValidations(t *testing.T) {
	ui := cli.NewMockUi()

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
		mockProject := &MockProject{tc.projectDetected}
		trellis := trellis.NewTrellis(mockProject)

		aliasCommand := NewAliasCommand(ui, trellis)

		code := aliasCommand.Run(tc.args)

		if code != tc.code {
			t.Errorf("%s: expected code %d to be %d", tc.name, code, tc.code)
		}

		combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

		if !strings.Contains(combined, tc.out) {
			t.Errorf("expected output %q to contain %q", combined, tc.out)
		}
	}
}

func TestIntegrationAlias(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	bin := os.Getenv("TEST_BINARY")
	if bin == "" {
		t.Error("TEST_BINARY not supplied")
	}
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Error(bin + "not exist")
	}

	dummy := os.Getenv("TEST_DUMMY")
	if dummy == "" {
		t.Error("TEST_DUMMY not supplied")
	}

	actualPath := path.Join(dummy, "site/wp-cli.trellis-alias.yml")

	os.Remove(actualPath)
	defer os.Remove(actualPath)

	alias := exec.Command(bin, "alias")
	alias.Dir = path.Join(dummy, "trellis")

	alias.Run()

	if _, err := os.Stat(actualPath); os.IsNotExist(err) {
		t.Error("wp-cli.trellis-alias.yml file not generated")
	}

	actualByte, _ := ioutil.ReadFile(actualPath)
	actual := string(actualByte)

	expectedByte, _ := ioutil.ReadFile("./testdata/expected/alias/wp-cli.trellis-alias.yml")
	expected := string(expectedByte)

	if actual != expected {
		t.Errorf("expected .wp-cli.trellis-alias.yml file \n%s to be \n%s", actual, expected)
	}
}
