package secrets

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Doc is an editable secrets document. It edits the YAML node tree, so
// comments and ordering survive `secrets init` and `secrets set`.
type Doc struct {
	root *yaml.Node // the top-level mapping
	doc  *yaml.Node
}

// ParseDoc parses plaintext secrets. Empty input is an empty document.
func ParseDoc(b []byte) (*Doc, error) {
	if len(bytes.TrimSpace(b)) == 0 {
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		return &Doc{root: m, doc: &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{m}}}, nil
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("secrets must be a YAML mapping")
	}
	return &Doc{root: doc.Content[0], doc: &doc}, nil
}

// Map decodes the document for lookups.
func (d *Doc) Map() (Map, error) {
	m := Map{}
	if err := d.root.Decode((*map[string]any)(&m)); err != nil {
		return nil, err
	}
	return m, nil
}

// Marshal encodes the document.
func (d *Doc) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(d.doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func child(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// Get returns the scalar at path.
func (d *Doc) Get(path ...string) (string, bool) {
	n := d.root
	for _, k := range path {
		if n.Kind != yaml.MappingNode {
			return "", false
		}
		if n = child(n, k); n == nil {
			return "", false
		}
	}
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	return n.Value, true
}

// Set stores value at path, creating intermediate mappings. An existing
// value is replaced only if overwrite is set. It reports whether the
// document changed.
func (d *Doc) Set(path []string, value string, overwrite bool) (bool, error) {
	if len(path) == 0 || slicesContainEmpty(path) {
		return false, fmt.Errorf("invalid secret path %q", strings.Join(path, "/"))
	}
	n := d.root
	for i, k := range path {
		last := i == len(path)-1
		c := child(n, k)
		if c == nil {
			c = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			if last {
				c = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			}
			n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, c)
			if last {
				return true, nil
			}
		} else if last {
			if c.Kind != yaml.ScalarNode {
				return false, fmt.Errorf("%s is a section, not a value", strings.Join(path, "/"))
			}
			if !overwrite || (c.Value == value && c.Tag == "!!str") {
				return false, nil
			}
			c.Value, c.Tag, c.Style = value, "!!str", 0
			return true, nil
		} else if c.Kind != yaml.MappingNode {
			return false, fmt.Errorf("%s is a value, not a section", strings.Join(path[:i+1], "/"))
		}
		n = c
	}
	return false, nil
}

func slicesContainEmpty(path []string) bool {
	for _, p := range path {
		if p == "" {
			return true
		}
	}
	return false
}

// EnsureSite adds any missing generated secrets (database password and
// WordPress salts) for site and returns their paths. Existing values are
// never replaced.
func (d *Doc) EnsureSite(site string) ([]string, error) {
	var added []string
	gen := func(n int, path ...string) error {
		if _, ok := d.Get(path...); ok {
			return nil
		}
		v, err := Random(n)
		if err != nil {
			return err
		}
		if _, err := d.Set(path, v, false); err != nil {
			return err
		}
		added = append(added, strings.Join(path, "/"))
		return nil
	}
	if err := gen(32, "sites", site, "db_password"); err != nil {
		return nil, err
	}
	for _, k := range SaltKeys {
		if err := gen(64, "sites", site, "salts", k); err != nil {
			return nil, err
		}
	}
	return added, nil
}
