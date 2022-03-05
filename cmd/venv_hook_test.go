package cmd

import (
	"fmt"
	"os"
	"testing"

	"github.com/mitchellh/cli"
	"github.com/roots/trellis-cli/trellis"
)

func TestVenvHookRunActivatesEnv(t *testing.T) {
	os.Unsetenv(trellis.OldPathEnvName)
	defer trellis.LoadFixtureProject(t)()

	ui := cli.NewMockUi()
	tp := trellis.NewTrellis()
	venvHookCommand := &VenvHookCommand{ui, tp}

	code := venvHookCommand.Run([]string{})

	if code != 0 {
		t.Errorf("expected code %d to be %d", code, 0)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

	venv := fmt.Sprintf("export %s=\"%s\"", trellis.VenvEnvName, tp.Virtualenv.Path)
	oldPath := fmt.Sprintf("export %s=\"%s\"", trellis.OldPathEnvName, tp.Virtualenv.OldPath)
	path := fmt.Sprintf("export %s=\"%s\":\"%s\"", trellis.PathEnvName, tp.Virtualenv.BinPath, tp.Virtualenv.OldPath)

	expected := fmt.Sprintf("%s\n%s\n%s\n", venv, oldPath, path)

	if combined != expected {
		t.Errorf("expected output %s to be %s", combined, expected)
	}
}

func TestVenvHookRunDeactivatesEnv(t *testing.T) {
	t.Setenv(trellis.OldPathEnvName, "foo")

	ui := cli.NewMockUi()
	trellis := trellis.NewMockTrellis(false)
	venvHookCommand := &VenvHookCommand{ui, trellis}

	code := venvHookCommand.Run([]string{})

	if code != 0 {
		t.Errorf("expected code %d to be %d", code, 0)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

	expected := `unset VIRTUAL_ENV
unset PRE_TRELLIS_PATH
export PATH=foo
`

	if combined != expected {
		t.Errorf("expected output %s to be %s", combined, expected)
	}
}

func TestVenvHookRunWithoutProject(t *testing.T) {
	os.Unsetenv(trellis.OldPathEnvName)

	ui := cli.NewMockUi()
	trellis := trellis.NewMockTrellis(false)
	venvHookCommand := &VenvHookCommand{ui, trellis}

	code := venvHookCommand.Run([]string{})

	if code != 0 {
		t.Errorf("expected code %d to be %d", code, 0)
	}

	combined := ui.OutputWriter.String() + ui.ErrorWriter.String()

	if combined != "" {
		t.Errorf("expected output %s to be empty", combined)
	}
}
