package config

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const FileName = "trellis.yml"

// Config is a parsed trellis.yml. It keeps the generic document so
// environments can be resolved with the single merge rule.
type Config struct {
	File File
	doc  map[string]any
}

// Find walks up from dir looking for trellis.yml and returns its path.
func Find(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		p := filepath.Join(dir, FileName)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("%s not found (searched upwards from the current directory)", FileName)
		}
		dir = parent
	}
}

// Load reads and parses path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Parse decodes trellis.yml strictly.
func Parse(b []byte) (*Config, error) {
	var f File
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, err
	}
	if f.Version != 1 {
		return nil, fmt.Errorf("version: must be 1 (got %d)", f.Version)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return &Config{File: f, doc: doc}, nil
}

// EnvironmentNames returns the defined environments, sorted.
func (c *Config) EnvironmentNames() []string {
	return slices.Sorted(maps.Keys(c.File.Environments))
}

var serverOnlyEnvKeys = []string{"nodes", "sites", "database", "access"}

// Resolve merges, defaults and validates environment env.
func (c *Config) Resolve(env string) (*Environment, error) {
	if _, ok := c.File.Environments[env]; !ok {
		return nil, fmt.Errorf("environment %q is not defined (have: %s)", env, strings.Join(c.EnvironmentNames(), ", "))
	}
	envDoc := asMap(asMap(c.doc["environments"])[env])

	// Server settings: defaults → environment (minus the env-only keys).
	envServer := maps.Clone(envDoc)
	for _, k := range serverOnlyEnvKeys {
		delete(envServer, k)
	}
	var e Environment
	if err := decodeInto(merge(asMap(c.doc["defaults"]), envServer), &e.Server); err != nil {
		return nil, fmt.Errorf("environments.%s: %w", env, err)
	}
	if err := decodeInto(merge(asMap(c.doc["access"]), asMap(envDoc["access"])), &e.Access); err != nil {
		return nil, fmt.Errorf("environments.%s.access: %w", env, err)
	}

	raw := c.File.Environments[env]
	e.Name = env
	e.Nodes = slices.Clone(raw.Nodes)
	e.Database = raw.Database

	// Sites: sites.<s> → environments.<env>.sites.<s>.
	topSites := asMap(c.doc["sites"])
	envSites := asMap(envDoc["sites"])
	var errs []error
	for name := range envSites {
		if _, ok := topSites[name]; !ok {
			errs = append(errs, fmt.Errorf("environments.%s.sites.%s: site is not defined under top-level sites", env, name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(topSites)) {
		override, present := envSites[name]
		if present && override == nil {
			continue // explicitly excluded from this environment
		}
		var s Site
		if err := decodeInto(merge(asMap(topSites[name]), asMap(override)), &s); err != nil {
			errs = append(errs, fmt.Errorf("sites.%s (%s): %w", name, env, err))
			continue
		}
		s.Name, s.EnvName = name, env
		e.Sites = append(e.Sites, s)
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}

	applyDefaults(&e)
	if err := validate(&e); err != nil {
		return nil, err
	}
	return &e, nil
}

// merge implements the single merge rule: maps deep-merge, everything else
// (scalars, lists) is replaced, nil removes the key.
func merge(base, over map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(over))
	maps.Copy(out, base)
	for k, v := range over {
		if v == nil {
			delete(out, k)
			continue
		}
		bm, bok := out[k].(map[string]any)
		om, ook := v.(map[string]any)
		if bok && ook {
			out[k] = merge(bm, om)
			continue
		}
		out[k] = v
	}
	return out
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

// decodeInto round-trips a generic map through YAML into a typed value.
// Types were already validated by the strict decode in Parse.
func decodeInto(m map[string]any, out any) error {
	if len(m) == 0 {
		return nil
	}
	b, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	return dec.Decode(out)
}
