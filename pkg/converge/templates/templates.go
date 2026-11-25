// Package templates holds Trellis's built-in configuration templates and
// renders them, honouring ejected overrides from the project.
package templates

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path"
	"slices"
	"strings"
	"sync"
	"text/template"
)

//go:embed files
var embedded embed.FS

const ext = ".tmpl"

var builtin = func() fs.FS {
	sub, err := fs.Sub(embedded, "files")
	if err != nil {
		panic(err)
	}
	return sub
}()

// Names lists built-in template names (e.g. "nginx/site.conf").
func Names() []string { return slices.Clone(builtinNames()) }

var builtinNames = sync.OnceValue(func() []string { return list(builtin) })

// list returns the template names (without extension) under fsys.
func list(fsys fs.FS) []string {
	var names []string
	_ = fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(p, ext) {
			names = append(names, strings.TrimSuffix(p, ext))
		}
		return nil
	})
	return names
}

// Source returns a built-in template's source.
func Source(name string) ([]byte, error) {
	b, err := fs.ReadFile(builtin, name+ext)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("unknown template %q (available: %s)", name, strings.Join(builtinNames(), ", "))
	}
	return b, err
}

// SHA256 is the hash of a built-in template's source.
func SHA256(name string) (string, error) {
	b, err := Source(name)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// KV is an ordered key/value pair for rendering maps deterministically.
type KV struct{ Key, Value string }

// Sorted returns m as KVs sorted by key.
func Sorted(m map[string]string) []KV {
	out := make([]KV, 0, len(m))
	for _, k := range slices.Sorted(maps.Keys(m)) {
		out = append(out, KV{k, m[k]})
	}
	return out
}

var funcs = template.FuncMap{
	"join": func(s []string, sep string) string { return strings.Join(s, sep) },
}

// Set renders templates. Ejected templates in overrides (paths like
// "nginx/site.conf.tmpl") take precedence over built-ins. Parsed templates
// are cached, so rendering a template per site parses it once.
type Set struct {
	overrides fs.FS
	ejected   map[string]bool

	mu    sync.Mutex
	cache map[string]*template.Template
}

// NewSet returns a Set using overrides (may be nil). The overrides are
// listed once here; later lookups don't touch the filesystem.
func NewSet(overrides fs.FS) *Set {
	s := &Set{overrides: overrides, ejected: map[string]bool{}, cache: map[string]*template.Template{}}
	if overrides != nil {
		for _, name := range list(overrides) {
			s.ejected[name] = true
		}
	}
	return s
}

// Ejected reports whether name is overridden by the project.
func (s *Set) Ejected(name string) bool { return s.ejected[name] }

func (s *Set) get(name string) (*template.Template, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.cache[name]; ok {
		return t, nil
	}
	var src []byte
	var err error
	if s.Ejected(name) {
		src, err = fs.ReadFile(s.overrides, name+ext)
	} else {
		src, err = Source(name)
	}
	if err != nil {
		return nil, err
	}
	t, err := Parse(name, string(src))
	if err != nil {
		return nil, err
	}
	s.cache[name] = t
	return t, nil
}

// Render executes template name with data.
func (s *Set) Render(name string, data any) (string, error) {
	t, err := s.get(name)
	if err != nil {
		return "", err
	}
	return Execute(t, data)
}

// Parse parses a template with the standard function map. Missing keys are
// errors so typos in ejected templates fail compilation, not the server.
func Parse(name, src string) (*template.Template, error) {
	t, err := template.New(path.Base(name)).Funcs(funcs).Option("missingkey=error").Parse(src)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", name, err)
	}
	return t, nil
}

// Execute renders t, normalising the result to end with one newline.
func Execute(t *template.Template, data any) (string, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template %s: %w", t.Name(), err)
	}
	return strings.TrimRight(buf.String(), "\n") + "\n", nil
}
