package plugin

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFind(t *testing.T) {
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

	cases := []struct {
		name             string
		validPrefixes    []string
		searchPaths      []string
		coreRootCommands []string
		pluginFileName   string
		pluginFileMode   os.FileMode
		out              map[string]string
	}{
		{
			"plugin_root_command",
			[]string{"trellis"},
			[]string{tempDir},
			[]string{"foo", "bar"},
			"trellis-xxx",
			0111,
			map[string]string{
				"xxx": filepath.Join(tempDir, "trellis-xxx"),
			},
		},
		{
			"under_core_root_command",
			[]string{"trellis"},
			[]string{tempDir},
			[]string{"foo", "bar"},
			"trellis-xxx-bar",
			0111,
			map[string]string{
				"xxx bar": filepath.Join(tempDir, "trellis-xxx-bar"),
			},
		},
		{
			"same_as_core_root_command",
			[]string{"trellis"},
			[]string{tempDir},
			[]string{"foo", "bar"},
			"trellis-bar",
			0111,
			map[string]string{},
		},
		{
			"under_core_root_command",
			[]string{"trellis"},
			[]string{tempDir},
			[]string{"foo", "bar"},
			"trellis-bar-xxx",
			0111,
			map[string]string{},
		},
		{
			"invalid_prefix",
			[]string{"trellis"},
			[]string{tempDir},
			[]string{"foo", "bar"},
			"not-trellis-xxx",
			0111,
			map[string]string{},
		},
		{
			"empty_search_path",
			[]string{"trellis"},
			[]string{""},
			[]string{"foo", "bar"},
			"trellis-xxx",
			0111,
			map[string]string{},
		},
		{
			"empty_and_non_empty_search_paths",
			[]string{"trellis"},
			[]string{tempDir, "", " "},
			[]string{"foo", "bar"},
			"trellis-xxx",
			0111,
			map[string]string{
				"xxx": filepath.Join(tempDir, "trellis-xxx"),
			},
		},
		{
			"duplicated_search_paths",
			[]string{"trellis"},
			[]string{tempDir, tempDir},
			[]string{"foo", "bar"},
			"trellis-xxx",
			0111,
			map[string]string{
				"xxx": filepath.Join(tempDir, "trellis-xxx"),
			},
		},
		{
			"not_exist_search_path",
			[]string{"trellis"},
			[]string{tempDir + "not_exist_search_path"},
			[]string{"foo", "bar"},
			"trellis-xxx",
			0111,
			map[string]string{},
		},
		{
			"non_executable",
			[]string{"trellis"},
			[]string{tempDir},
			[]string{"foo", "bar"},
			"trellis-xxx",
			0666,
			map[string]string{},
		},
	}

	for _, tc := range cases {
		// cleanup files from previous test case.
		os.RemoveAll(tempDir)
		os.MkdirAll(tempDir, 0700)

		if _, err := createTempFile(tc.pluginFileName, tc.pluginFileMode); err != nil {
			t.Fatalf("unexpected error creating plugin file: %v", err)
		}

		pluginFinder := finder{
			validPrefixes:    tc.validPrefixes,
			searchPaths:      tc.searchPaths,
			coreRootCommands: tc.coreRootCommands,
		}

		output := pluginFinder.find()

		if !reflect.DeepEqual(output, tc.out) {
			t.Errorf("%s: expected output %v to be %v", tc.name, output, tc.out)
		}
	}
}

func TestIsExecutable(t *testing.T) {
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

	createTempFile := func(mode os.FileMode) (*os.File, error) {
		file, err := ioutil.TempFile(tempDir, "trellis-")
		if err != nil {
			return nil, err
		}

		err = os.Chmod(file.Name(), mode)
		if err != nil {
			return nil, err
		}

		return file, nil
	}

	cases := []struct {
		name string
		mode os.FileMode
		out  bool
	}{
		{
			"valid-0100",
			0100,
			true,
		},
		{
			"valid-0010",
			0010,
			true,
		},
		{
			"valid-0001",
			0001,
			true,
		},
		{
			"valid-0111",
			0111,
			true,
		},
		{
			"invalid-0666",
			0666,
			false,
		},
	}

	for _, tc := range cases {
		file, err := createTempFile(tc.mode)
		if err != nil {
			t.Fatalf("unexpected error creating plugin file: %v", err)
		}

		output := isExecutable(file.Name())

		if output != tc.out {
			t.Errorf("%s: expected output %t to be %t", tc.name, output, tc.out)
		}
	}
}

func TestHasValidPrefix(t *testing.T) {
	cases := []struct {
		name          string
		filepath      string
		validPrefixes []string
		out           bool
	}{
		{
			"valid_root_command",
			"trellis-xxx",
			[]string{"trellis"},
			true,
		},
		{
			"valid_subcommand",
			"trellis-xxx-yyy",
			[]string{"trellis"},
			true,
		},
		{
			"invalid_prefix",
			"xxx-yyy",
			[]string{"trellis"},
			false,
		},
		{
			"prefix_without_hyphen",
			"trellisxxx",
			[]string{"trellis"},
			false,
		},
		{
			"prefix_only",
			"trellis",
			[]string{"trellis"},
			false,
		},
		{
			"prefix_as_substring",
			"xxx-trellis-yyy",
			[]string{"trellis"},
			false,
		},
		{
			"end_with_prefix",
			"xxx-trellis",
			[]string{"trellis"},
			false,
		},
		{
			"underscore_as_separator",
			"trellis_xxx",
			[]string{"trellis"},
			false,
		},
	}

	for _, tc := range cases {
		output := hasValidPrefix(tc.filepath, tc.validPrefixes)

		if output != tc.out {
			t.Errorf("%s: expected output %t to be %t", tc.name, output, tc.out)
		}
	}
}

func TestIsUnderCoreRootCommands(t *testing.T) {
	cases := []struct {
		name             string
		filepath         string
		coreRootCommands []string
		out              bool
	}{
		{
			"same_as_core_root_command",
			"trellis-foo",
			[]string{"foo", "bar"},
			true,
		},
		{
			"subcommand_under_core_root_command",
			"trellis-foo-xxx",
			[]string{"foo", "bar"},
			true,
		},
		{
			"different_root_command",
			"trellis-xxx",
			[]string{"foo", "bar"},
			false,
		},
		{
			"subcommand_under_different_root_command",
			"trellis-xxx-foo",
			[]string{"foo", "bar"},
			false,
		},
	}

	for _, tc := range cases {
		output := isUnderCoreRootCommands(tc.filepath, tc.coreRootCommands)

		if output != tc.out {
			t.Errorf("%s: expected output %t to be %t", tc.name, output, tc.out)
		}
	}
}
