package trellis

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roots/trellis-cli/command"
)

func TestNewVirtualenv(t *testing.T) {
	venv := NewVirtualenv("trellis")
	path := "trellis/virtualenv"
	binPath := "trellis/virtualenv/bin"
	oldPath := os.Getenv("PATH")

	if venv.Path != path {
		t.Errorf("expected Path to be %s, but got %s", venv.Path, path)
	}

	if venv.BinPath != binPath {
		t.Errorf("expected BinPath to be %s, but got %s", venv.BinPath, binPath)
	}

	if venv.OldPath != oldPath {
		t.Errorf("expected OldPath to be %s, but got %s", venv.OldPath, oldPath)
	}
}

func TestActivateSetsEnv(t *testing.T) {
	venv := NewVirtualenv("trellis")
	originalPath := os.Getenv("PATH")

	venv.Activate()

	if os.Getenv("VIRTUAL_ENV") != "trellis/virtualenv" {
		t.Error("expected VIRTUAL_ENV env var to set")
	}

	if os.Getenv("PATH") != fmt.Sprintf("trellis/virtualenv/bin:%s", originalPath) {
		t.Error("expected PATH to contain bin path")
	}

	if venv.OldPath != os.Getenv("PRE_TRELLIS_PATH") {
		t.Error("expected OldPath to be the original PATH")
	}

	venv.Deactivate()
}

func TestActivateIsIdempotent(t *testing.T) {
	venv := NewVirtualenv("trellis")
	originalPath := os.Getenv("PATH")

	venv.Activate()
	venv.Activate()

	if os.Getenv("VIRTUAL_ENV") != "trellis/virtualenv" {
		t.Error("expected VIRTUAL_ENV env var to set")
	}

	if os.Getenv("PATH") != fmt.Sprintf("trellis/virtualenv/bin:%s", originalPath) {
		t.Error("expected PATH to contain bin path")
	}

	if venv.OldPath != originalPath {
		t.Error("expected OldPath to be the original PATH")
	}

	venv.Deactivate()
}

func TestActive(t *testing.T) {
	venv := NewVirtualenv("trellis")

	if venv.Active() {
		t.Error("expected virtualenv to be inactive")
	}

	venv.Activate()

	if !venv.Active() {
		t.Error("expected virtualenv to be active")
	}
}

func TestDeactive(t *testing.T) {
	venv := NewVirtualenv("trellis")
	venv.Activate()
	venv.Deactivate()

	if os.Getenv("VIRTUALENV") != "" {
		t.Error("Expected VIRTUALENV to be empty")
	}

	if os.Getenv("PATH") != venv.OldPath {
		t.Error("Expected PATH to be reset")
	}

	if os.Getenv("PRE_TRELLIS_PATH") != "" {
		t.Error("Expected PRE_TRELLIS_PATH to be empty")
	}
}

func TestInitialized(t *testing.T) {
	tempDir := t.TempDir()

	venv := NewVirtualenv(tempDir)

	if venv.Initialized() {
		t.Error("Expected to be uniniatlized")
	}

	if err := os.MkdirAll(venv.BinPath, os.ModePerm); err != nil {
		t.Fatal(err)
	}
	testCreateFile(t, filepath.Join(venv.BinPath, "python"))()
	testCreateFile(t, filepath.Join(venv.BinPath, "pip"))()

	if !venv.Initialized() {
		t.Error("Expected to be initialized")
	}
}

func TestInstalled(t *testing.T) {
	t.Setenv("PATH", "")
	t.Setenv("XDG_CONFIG_HOME", "none")

	venv := NewVirtualenv("foo")

	ok, _ := venv.Installed()

	if ok {
		t.Error("Expected to be uninstalled")
	}
}

func TestInstalledPython3WithEnsurepip(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PATH", tempDir)

	pythonPath := filepath.Join(tempDir, "python3")
	if _, err := os.OpenFile(pythonPath, os.O_CREATE, 0555); err != nil {
		t.Fatal(err)
	}

	venv := NewVirtualenv(tempDir)

	var output bytes.Buffer

	mockExecCommand := func(command string, args []string) *exec.Cmd {
		cs := []string{"-test.run=TestEnsurePipSuccessHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Stderr = &output
		cmd.Stdout = &output
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}

	command.Mock(mockExecCommand)
	defer command.Restore()

	ok, cmd := venv.Installed()

	if !ok {
		t.Error("Expected to be installed")
	}

	expected := "python3 -m venv"
	actual := cmd.String()

	if !strings.Contains(actual, expected) {
		t.Errorf("Expected command incorrect.\nexpected: %s\ngot: %s", expected, actual)
	}
}

func TestInstalledPython3WithoutEnsurepip(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PATH", tempDir)

	pythonPath := filepath.Join(tempDir, "python3")
	if _, err := os.OpenFile(pythonPath, os.O_CREATE, 0555); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer

	mockExecCommand := func(command string, args []string) *exec.Cmd {
		cs := []string{"-test.run=TestEnsurePipFailureHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Stderr = &output
		cmd.Stdout = &output
		cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
		return cmd
	}

	command.Mock(mockExecCommand)
	defer command.Restore()

	venv := NewVirtualenv(tempDir)

	ok, _ := venv.Installed()

	if ok {
		t.Error("Expected not to be installed")
	}
}

func TestInstalledVirtualenv(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("PATH", tempDir)

	venvPath := filepath.Join(tempDir, "virtualenv")
	if _, err := os.OpenFile(venvPath, os.O_CREATE, 0555); err != nil {
		t.Fatal(err)
	}

	venv := NewVirtualenv(tempDir)

	ok, cmd := venv.Installed()

	if !ok {
		t.Error("Expected to be installed")
	}

	if strings.Join(cmd.Args, " ") != venvPath {
		t.Error("Expected args incorrect")
	}
}

func TestReplaceShebang(t *testing.T) {
	venv := NewVirtualenv("/trellis")

	cases := []struct {
		name   string
		input  string
		output string
	}{
		{
			"no_match",
			`#!/trellis/python
next line
`,
			`#!/trellis/python
next line
`,
		},
		{
			"first_line_match",
			`#!/trellis/virtualenv/bin/python
next line
`,
			`#!/bin/sh
'''exec' "/trellis/virtualenv/bin/python" "$0" "$@"
' '''
next line
`,
		},
		{
			"other_line_match",
			`first line
#!/trellis/virtualenv/bin/python
next line
`,
			`first line
#!/trellis/virtualenv/bin/python
next line
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := strings.NewReader(tc.input)
			b, _ := venv.replaceShebang(r)

			if b.String() != tc.output {
				t.Errorf("%s\n expected output: %s\ngot: %s", tc.name, tc.output, b.String())
			}
		})
	}
}

func TestUpdateBinShebangsNoSpaces(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "virtualenv", "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	venv := NewVirtualenv(dir)
	content := `#!/trellis/virtualenv/bin/python\n`
	path := filepath.Join(venv.BinPath, "foo")

	if err := os.WriteFile(path, []byte(content), os.ModePerm); err != nil {
		t.Fatal(err)
	}

	_ = venv.UpdateBinShebangs("foo*")

	output, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(output) != content {
		t.Errorf("expected output: %s\ngot: %s", output, content)
	}
}

func TestUpdateBinShebangsWithSpaces(t *testing.T) {
	t.SkipNow()
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "virtualenv", "bin"), 0755); err != nil {
		t.Fatal(err)
	}

	venv := NewVirtualenv(dir)

	cases := []struct {
		name         string
		filename     string
		contents     string
		expectChange bool
	}{
		{
			name:         "match",
			filename:     filepath.Join(venv.BinPath, "foo"),
			contents:     fmt.Sprintf("#!%s/python\nnext line", venv.BinPath),
			expectChange: true,
		},
		{
			name:         "no_matching_shebang",
			filename:     filepath.Join(venv.BinPath, "foo-bar"),
			contents:     "#!/not a match/python",
			expectChange: false,
		},
		{
			name:         "no_glob_match",
			filename:     filepath.Join(venv.BinPath, "ansible"),
			contents:     fmt.Sprintf("#!%s/python\nnext line", venv.BinPath),
			expectChange: false,
		},
	}

	for _, tc := range cases {
		if err := os.WriteFile(tc.filename, []byte(tc.contents), os.ModePerm); err != nil {
			t.Fatal(err)
		}
	}

	_ = venv.UpdateBinShebangs("foo*")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := os.ReadFile(tc.filename)

			if err != nil {
				t.Fatal(err)
			}

			if string(output) == tc.contents && tc.expectChange {
				t.Errorf("%s\n expected output: %s\ngot: %s", tc.name, tc.contents, output)
			}
		})
	}
}

func testCreateFile(t *testing.T, path string) func() {
	file, err := os.Create(path)

	if err != nil {
		t.Fatalf("err: %s", err)
	}

	return func() { _ = file.Close() }
}

func TestEnsurePipSuccessHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprint(os.Stdout, strings.Join(os.Args[3:], " "))
	os.Exit(0)
}

func TestEnsurePipFailureHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	fmt.Fprint(os.Stderr, strings.Join(os.Args[3:], " "))
	os.Exit(1)
}
