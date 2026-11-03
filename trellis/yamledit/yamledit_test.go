package yamledit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripTrellisFiles(t *testing.T) {
	files, _ := filepath.Glob("../testdata/trellis/group_vars/*/*.yml")
	if len(files) == 0 {
		t.Fatal("no fixture files found")
	}

	for _, file := range files {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			if strings.HasPrefix(string(data), "$ANSIBLE_VAULT") {
				t.Skip("encrypted vault file")
			}

			assertRoundTrip(t, string(data))
		})
	}
}

func TestRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{"blank_lines_between_keys", "a: 1\n\nb: 2\n\n\nc: 3\n"},
		{"blank_line_before_comment", "a: 1\n\n# comment\nb: 2\n"},
		{"doc_comment_without_blank_line", "# comment\na: 1\n\nb: 2\n"},
		{"doc_comment_with_blank_line", "# comment\n\na: 1\n\nb: 2\n"},
		{"nested_blank_lines", "a:\n  b: 1\n\n  c: 2\n"},
		{"sequence_items", "a:\n  - name: x\n    value: 1\n\n  - name: y\n    value: 2\n"},
		{"literal_block_with_blank_line", "a: |\n  line 1\n\n  line 2\nb: 2\n"},
		{"inline_comments", "a: 1 # one\nb: \"two\" # two\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertRoundTrip(t, tc.yaml)
		})
	}
}

func TestParseEmpty(t *testing.T) {
	for _, input := range []string{"", "# only a comment\n"} {
		doc, err := Parse([]byte(input))
		if err != nil {
			t.Fatal(err)
		}

		if err := doc.Set("value", "key"); err != nil {
			t.Errorf("Set on %q: %v", input, err)
		}
	}
}

func TestLookupAndDecode(t *testing.T) {
	doc := mustParse(t, "sites:\n  example.com:\n    enabled: true\n")

	if node := doc.Lookup("sites", "example.com", "enabled"); node == nil || node.Value != "true" {
		t.Errorf("Lookup returned %v", node)
	}

	if node := doc.Lookup("sites", "missing"); node != nil {
		t.Errorf("expected nil for missing key, got %v", node)
	}

	if node := doc.Lookup("sites", "example.com", "enabled", "deeper"); node != nil {
		t.Errorf("expected nil when traversing a scalar, got %v", node)
	}

	var site struct {
		Enabled bool `yaml:"enabled"`
	}

	if err := doc.Decode(&site, "sites", "example.com"); err != nil {
		t.Fatal(err)
	}

	if !site.Enabled {
		t.Error("expected decoded site to be enabled")
	}

	if err := doc.Decode(&site, "sites", "missing"); err == nil {
		t.Error("expected error decoding missing key")
	}
}

func TestSet(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		path     []string
		value    any
		expected string
	}{
		{
			"keeps_double_quotes_and_comments",
			"# head\nkey: \"generateme\" # line\nother: 1\n",
			[]string{"key"},
			"secret",
			"# head\nkey: \"secret\" # line\nother: 1\n",
		},
		{
			"keeps_single_quotes",
			"key: 'old'\n",
			[]string{"key"},
			"new",
			"key: 'new'\n",
		},
		{
			"plain_scalar_quotes_when_needed",
			"key: old\n",
			[]string{"key"},
			"true",
			"key: \"true\"\n",
		},
		{
			"replaces_sequence",
			"hosts:\n  - a\n  - b\nnext: 1\n",
			[]string{"hosts"},
			[]map[string]any{{"canonical": "c"}},
			"hosts:\n  - canonical: c\nnext: 1\n",
		},
		{
			"appends_missing_key",
			"a: 1\n",
			[]string{"b"},
			2,
			"a: 1\nb: 2\n",
		},
		{
			"creates_missing_mappings",
			"sites:\n  one:\n    x: 1\n",
			[]string{"sites", "two", "env", "x"},
			"z",
			"sites:\n  one:\n    x: 1\n  two:\n    env:\n      x: z\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := mustParse(t, tc.input)

			if err := doc.Set(tc.value, tc.path...); err != nil {
				t.Fatal(err)
			}

			assertOutput(t, doc, tc.expected)
		})
	}
}

func TestSetErrors(t *testing.T) {
	doc := mustParse(t, "a: 1\nlist:\n  - x\n")

	if err := doc.Set("x"); err == nil {
		t.Error("expected error for empty path")
	}

	if err := doc.Set("x", "a", "b"); err == nil {
		t.Error("expected error when traversing a scalar")
	}

	if err := doc.Set("x", "list", "1"); err == nil {
		t.Error("expected error for out of range sequence index")
	}

	if err := doc.Set("x", "list", "name"); err == nil {
		t.Error("expected error for non-numeric sequence index")
	}
}

func TestSequenceItems(t *testing.T) {
	doc := mustParse(t, "users:\n  - name: a\n    salt: \"generateme\"\n  - name: b\n")

	if node := doc.Lookup("users", "1", "name"); node == nil || node.Value != "b" {
		t.Errorf("Lookup by index returned %v", node)
	}

	if node := doc.Lookup("users", "2"); node != nil {
		t.Errorf("expected nil for out of range index, got %v", node)
	}

	if err := doc.Set("secret", "users", "0", "salt"); err != nil {
		t.Fatal(err)
	}

	if err := doc.Set("pw", "users", "1", "password"); err != nil {
		t.Fatal(err)
	}

	if err := doc.RenameKey("x", "users", "0"); err == nil {
		t.Error("expected error renaming a sequence item")
	}

	assertOutput(t, doc, "users:\n  - name: a\n    salt: \"secret\"\n  - name: b\n    password: pw\n")
}

func TestDelete(t *testing.T) {
	doc := mustParse(t, "a: 1\nsite:\n  repo: x # comment\n  branch: main\n")

	if !doc.Delete("site", "repo") {
		t.Error("expected nested delete to succeed")
	}

	if !doc.Delete("a") {
		t.Error("expected top-level delete to succeed")
	}

	if doc.Delete("site", "missing") || doc.Delete("missing", "key") || doc.Delete() {
		t.Error("expected deleting missing keys to return false")
	}

	assertOutput(t, doc, "site:\n  branch: main\n")
}

func TestRenameKey(t *testing.T) {
	doc := mustParse(t, "sites:\n  # the site\n  example.com: # inline\n    a: 1\n  other.com:\n    b: 2\n")

	path := []string{"sites", "example.com"}
	if err := doc.RenameKey("mysite.com", path...); err != nil {
		t.Fatal(err)
	}

	if path[1] != "example.com" {
		t.Errorf("RenameKey modified the caller's path: %v", path)
	}

	assertOutput(t, doc, "sites:\n  # the site\n  mysite.com: # inline\n    a: 1\n  other.com:\n    b: 2\n")

	if err := doc.RenameKey("other.com", "sites", "mysite.com"); err == nil {
		t.Error("expected error renaming to an existing key")
	}

	if err := doc.RenameKey("x", "sites", "missing"); err == nil {
		t.Error("expected error renaming a missing key")
	}
}

func TestHeadComment(t *testing.T) {
	doc := mustParse(t, "# original\n\na: 1\n")

	doc.SetHeadComment("# Created by test\n" + doc.HeadComment())

	assertOutput(t, doc, "# Created by test\n# original\n\na: 1\n")
}

func TestSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.yml")

	if err := os.WriteFile(path, []byte("a: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	doc, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := doc.Set(2, "a"); err != nil {
		t.Fatal(err)
	}

	if err := doc.Save(path); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "a: 2\n" {
		t.Errorf("unexpected file contents: %q", data)
	}

	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Errorf("expected permissions 0600, got %o", info.Mode().Perm())
	}
}

func mustParse(t *testing.T, input string) *Document {
	t.Helper()

	doc, err := Parse([]byte(input))
	if err != nil {
		t.Fatal(err)
	}

	return doc
}

func assertRoundTrip(t *testing.T, input string) {
	t.Helper()
	assertOutput(t, mustParse(t, input), input)
}

func assertOutput(t *testing.T, doc *Document, expected string) {
	t.Helper()

	out, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if strings.TrimRight(string(out), "\n") != strings.TrimRight(expected, "\n") {
		t.Errorf("expected:\n%s\ngot:\n%s", expected, out)
	}
}
