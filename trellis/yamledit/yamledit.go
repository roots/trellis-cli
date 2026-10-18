// Package yamledit makes targeted edits to YAML files while preserving
// comments, key order, and blank lines that a struct round-trip would lose.
package yamledit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const indent = 2

type Document struct {
	root *yaml.Node
}

func Parse(data []byte) (*Document, error) {
	var root yaml.Node

	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}

	if root.Kind == 0 {
		root.Kind = yaml.DocumentNode
	}

	if len(root.Content) == 0 {
		root.Content = []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}
	}

	preserveBlankLines(&root, strings.Split(string(data), "\n"))

	return &Document{root: &root}, nil
}

func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return Parse(data)
}

func (d *Document) Bytes() ([]byte, error) {
	var buf bytes.Buffer

	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(indent)

	if err := encoder.Encode(d.root); err != nil {
		return nil, err
	}

	if err := encoder.Close(); err != nil {
		return nil, err
	}

	return stripBlankLineIndentation(buf.Bytes()), nil
}

// Save writes the document to path, keeping the existing file's permissions.
func (d *Document) Save(path string) error {
	data, err := d.Bytes()
	if err != nil {
		return err
	}

	perm := os.FileMode(0644)
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	return os.WriteFile(path, data, perm)
}

// SetHeadComment replaces the comment at the top of the document.
func (d *Document) SetHeadComment(comment string) {
	d.root.HeadComment = comment
}

func (d *Document) HeadComment() string {
	return d.root.HeadComment
}

// Lookup returns the value node at the given key path, or nil if it doesn't exist.
func (d *Document) Lookup(path ...string) *yaml.Node {
	_, value, err := d.find(path)
	if err != nil {
		return nil
	}

	return value
}

// Decode decodes the value at the given key path into v.
func (d *Document) Decode(v any, path ...string) error {
	node := d.Lookup(path...)
	if node == nil {
		return fmt.Errorf("key not found: %s", formatPath(path))
	}

	return node.Decode(v)
}

// Set encodes value and stores it at the given key path, creating any missing
// mappings along the way. Comments on an existing value are kept, as is the
// quoting style when replacing a quoted scalar with another scalar.
func (d *Document) Set(value any, path ...string) error {
	if len(path) == 0 {
		return errors.New("path must not be empty")
	}

	var newNode yaml.Node
	if err := newNode.Encode(value); err != nil {
		return err
	}

	mapping := d.body()
	for i, key := range path[:len(path)-1] {
		_, next := findKey(mapping, key)

		if next == nil {
			next = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			appendKey(mapping, key, next)
		} else if next.Kind != yaml.MappingNode {
			return fmt.Errorf("%s is not a mapping", formatPath(path[:i+1]))
		}

		mapping = next
	}

	key := path[len(path)-1]
	_, existing := findKey(mapping, key)

	if existing == nil {
		appendKey(mapping, key, &newNode)
		return nil
	}

	if existing.Kind == yaml.ScalarNode && newNode.Kind == yaml.ScalarNode && isQuoted(existing.Style) {
		newNode.Style = existing.Style
	}

	newNode.HeadComment = existing.HeadComment
	newNode.LineComment = existing.LineComment
	newNode.FootComment = existing.FootComment
	*existing = newNode

	return nil
}

// Delete removes the key at the given path. It returns false if the key didn't exist.
func (d *Document) Delete(path ...string) bool {
	if len(path) == 0 {
		return false
	}

	parent := d.body()
	if len(path) > 1 {
		parent = d.Lookup(path[:len(path)-1]...)
	}

	if parent == nil || parent.Kind != yaml.MappingNode {
		return false
	}

	for i := 0; i < len(parent.Content)-1; i += 2 {
		if parent.Content[i].Value == path[len(path)-1] {
			parent.Content = append(parent.Content[:i], parent.Content[i+2:]...)
			return true
		}
	}

	return false
}

// RenameKey renames the key at the given path, keeping its position and comments.
func (d *Document) RenameKey(newKey string, path ...string) error {
	key, _, err := d.find(path)
	if err != nil {
		return err
	}

	if key.Value == newKey {
		return nil
	}

	newPath := slices.Concat(path[:len(path)-1], []string{newKey})
	if d.Lookup(newPath...) != nil {
		return fmt.Errorf("key already exists: %s", formatPath(newPath))
	}

	key.Value = newKey

	return nil
}

func (d *Document) body() *yaml.Node {
	return d.root.Content[0]
}

func (d *Document) find(path []string) (key *yaml.Node, value *yaml.Node, err error) {
	if len(path) == 0 {
		return nil, nil, errors.New("path must not be empty")
	}

	value = d.body()

	for i, k := range path {
		if value.Kind != yaml.MappingNode {
			return nil, nil, fmt.Errorf("%s is not a mapping", formatPath(path[:i]))
		}

		key, value = findKey(value, k)
		if key == nil {
			return nil, nil, fmt.Errorf("key not found: %s", formatPath(path[:i+1]))
		}
	}

	return key, value, nil
}

func findKey(mapping *yaml.Node, key string) (*yaml.Node, *yaml.Node) {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i], mapping.Content[i+1]
		}
	}

	return nil, nil
}

func appendKey(mapping *yaml.Node, key string, value *yaml.Node) {
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	mapping.Content = append(mapping.Content, keyNode, value)
}

func isQuoted(style yaml.Style) bool {
	return style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) != 0
}

func formatPath(path []string) string {
	return strings.Join(path, ".")
}

// preserveBlankLines works around yaml.v3 dropping blank lines between
// entries: the encoder emits a blank line for each leading newline in a head
// comment, so blank lines found in the source above a mapping key or sequence
// item are encoded into its head comment.
func preserveBlankLines(root *yaml.Node, lines []string) {
	handled := map[int]bool{}

	// The encoder already keeps the blank line after a document head comment.
	if root.HeadComment != "" && len(root.Content) > 0 {
		handled[root.Content[0].Line] = true
	}

	var walk func(node *yaml.Node)
	walk = func(node *yaml.Node) {
		switch node.Kind {
		case yaml.MappingNode:
			for i := 0; i < len(node.Content)-1; i += 2 {
				addBlankLines(node.Content[i], lines, handled)
				walk(node.Content[i+1])
			}
		case yaml.SequenceNode:
			for _, item := range node.Content {
				addBlankLines(item, lines, handled)
				walk(item)
			}
		default:
			for _, child := range node.Content {
				walk(child)
			}
		}
	}

	walk(root)
}

// Blank lines restored inside nested blocks are emitted with the block's
// indentation. The encoder never uses block scalar style for content with
// trailing spaces, so whitespace-only lines are always safe to empty.
func stripBlankLineIndentation(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))

	for i, line := range lines {
		if len(bytes.TrimLeft(line, " ")) == 0 {
			lines[i] = nil
		}
	}

	return bytes.Join(lines, []byte("\n"))
}

func addBlankLines(node *yaml.Node, lines []string, handled map[int]bool) {
	if node.Line == 0 || handled[node.Line] {
		return
	}

	handled[node.Line] = true

	start := node.Line
	if node.HeadComment != "" {
		start -= strings.Count(node.HeadComment, "\n") + 1
	}

	blanks := 0
	for i := start - 2; i >= 0 && strings.TrimSpace(lines[i]) == ""; i-- {
		blanks++
	}

	// Blank lines at the very start of the file aren't meaningful.
	if blanks == 0 || blanks == start-1 {
		return
	}

	node.HeadComment = strings.Repeat("\n", blanks) + node.HeadComment
}
