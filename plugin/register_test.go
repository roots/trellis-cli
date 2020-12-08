package plugin

import (
	"fmt"
	"github.com/mitchellh/cli"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"trellis-cli/cmd"
)

const spyCommand = `#!/usr/bin/env bash
echo "i am plugin command"
echo "total number of arguments passed: $#"
echo "values of all the arguments passed: $@"
`

func TestIntegrationPluginCommand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	bin := os.Getenv("TEST_BINARY")
	if bin == "" {
		t.Error("TEST_BINARY not supplied")
	}
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Error(bin + " not exist")
	}

	tempDir, err := ioutil.TempDir(os.TempDir(), "test-cmd-plugins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// cleanup
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			panic(fmt.Errorf("unexpected cleanup error: %v", err))
		}
	}()

	file, err := os.Create(filepath.Join(tempDir, "trellis-spy-foo"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := file.WriteString(spyCommand); err != nil {
		file.Close()
		t.Fatalf("unexpected error: %v", err)
	}
	file.Close()
	err = os.Chmod(file.Name(), 0111)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cases := []struct {
		name           string
		args           []string
		expectedStdOut []string
		expectedStdErr []string
	}{
		{
			"--help",
			[]string{"--help"},
			[]string{},
			[]string{
				"Available third party plugin commands are",
				"spy",
			},
		},
		{
			"spy",
			[]string{"spy"},
			[]string{},
			[]string{
				"This command is accessed by using one of the subcommands below",
				"foo",
				"Third party plugin: Forward command to trellis-spy-foo",
			},
		},
		{
			"spy foo",
			[]string{"spy", "foo"},
			[]string{
				"i am plugin command",
				"total number of arguments passed: 0",
				"values of all the arguments passed:",
			},
			[]string{},
		},
		{
			"spy foo --help",
			[]string{"spy", "foo", "--help"},
			[]string{
				"i am plugin command",
				"total number of arguments passed: 1",
				"values of all the arguments passed: --help",
			},
			[]string{},
		},
		{
			"spy foo --aaa bbb --ccc ddd",
			[]string{"spy", "foo", "--aaa", "bbb", "--ccc", "ddd"},
			[]string{
				"i am plugin command",
				"total number of arguments passed: 4",
				"values of all the arguments passed: --aaa bbb --ccc ddd",
			},
			[]string{},
		},
	}

	for _, tc := range cases {
		mockUi := cli.NewMockUi()
		spyCommand := cmd.CommandExecWithOutput(bin, tc.args, mockUi)
		spyCommand.Env = []string{"PATH=" + tempDir + ":" + os.ExpandEnv("$PATH")}

		spyCommand.Run()

		stdOut := mockUi.OutputWriter.String()
		for _, expected := range tc.expectedStdOut {
			if !strings.Contains(stdOut, expected) {
				t.Errorf("$ trellis %s: expected stdOut %q to contain %q", tc.name, stdOut, expected)
			}
		}
		stdErr := mockUi.ErrorWriter.String()
		for _, expected := range tc.expectedStdErr {
			if !strings.Contains(stdErr, expected) {
				t.Errorf("$ trellis %s: expected stdErr %q to contain %q", tc.name, stdErr, expected)
			}
		}
	}
}

func TestIntegrationPluginListInHelpFunc(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	bin := os.Getenv("TEST_BINARY")
	if bin == "" {
		t.Error("TEST_BINARY not supplied")
	}
	if _, err := os.Stat(bin); os.IsNotExist(err) {
		t.Error(bin + " not exist")
	}

	tempDir, err := ioutil.TempDir(os.TempDir(), "test-cmd-plugins")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// cleanup
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			panic(fmt.Errorf("unexpected cleanup error: %v", err))
		}
	}()

	createTempFile := func(name string, mode os.FileMode) (*os.File, error) {
		file, err := os.Create(filepath.Join(tempDir, name))
		if err != nil {
			return nil, err
		}

		err = os.Chmod(file.Name(), mode)
		if err != nil {
			return nil, err
		}

		return file, nil
	}

	if _, err := createTempFile("trellis-foo", 0111); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := createTempFile("trellis-bar", 0111); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mockUi := cli.NewMockUi()

	trellisCommand := cmd.CommandExecWithOutput(bin, []string{"--help"}, mockUi)
	trellisCommand.Env = []string{"PATH=" + tempDir + ":$PATH"}

	trellisCommand.Run()
	output := mockUi.ErrorWriter.String()

	expected := "Available third party plugin commands are"
	if !strings.Contains(output, expected) {
		t.Errorf("expected output %q to contain %q", output, expected)
	}
	for _, plugin := range []string{"foo", "bar"} {
		if !strings.Contains(output, plugin) {
			t.Errorf("expected output %q to contain %q", output, plugin)
		}
	}
}

func TestRootCommandsFor(t *testing.T) {
	cases := []struct {
		in  map[string]string
		out []string
	}{
		{
			map[string]string{
				"foo": "xxx",
			},
			[]string{"foo"},
		},
		{
			map[string]string{
				"foo": "xxx",
				"bar": "yyy",
			},
			[]string{"foo", "bar"},
		},
		{
			map[string]string{},
			[]string{},
		},
	}

	for _, tc := range cases {
		output := rootCommandsFor(reflect.ValueOf(tc.in))

		sort.Strings(output)
		sort.Strings(tc.out)
		if !reflect.DeepEqual(output, tc.out) {
			t.Errorf("expected output %v to be %v", output, tc.out)
		}
	}
}

func TestUnique(t *testing.T) {
	cases := []struct {
		in  []string
		out []string
	}{
		{
			[]string{"foo"},
			[]string{"foo"},
		},
		{
			[]string{"foo", "bar"},
			[]string{"foo", "bar"},
		},
		{
			[]string{"foo", "foo", "foo", "bar"},
			[]string{"foo", "bar"},
		},
		{
			[]string{"foo", "bar", "", " ", "foo", "bar", "", " "},
			[]string{"foo", "bar", "", " "},
		},
		{
			[]string{},
			[]string{},
		},
	}

	for _, tc := range cases {
		output := unique(tc.in)

		sort.Strings(output)
		sort.Strings(tc.out)
		if !reflect.DeepEqual(output, tc.out) {
			t.Errorf("expected output %v to be %v", output, tc.out)
		}
	}
}
