package fingerprint

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const absent = "\x00absent"

// Kind classifies a fact that at least one provisioner changed.
type Kind string

const (
	Same         Kind = "same"          // both set the same value
	Differ       Kind = "differ"        // both changed it, to different values
	OnlyTrellis  Kind = "only-trellis"  // converge left the baseline value
	OnlyConverge Kind = "only-converge" // Trellis left the baseline value
)

// Change is one compared fact.
type Change struct {
	Key                       string
	Kind                      Kind
	Before, Trellis, Converge string
	Accepted                  string // reason, if an expectation matched
}

// Expectation accepts known, intended differences.
type Expectation struct {
	Key    string `yaml:"key"` // regexp on the fact key
	Reason string `yaml:"reason"`
	re     *regexp.Regexp
}

// Config holds noise filters and accepted differences.
type Config struct {
	// Ignore are regexps for volatile facts (counters, timestamps).
	Ignore   []string      `yaml:"ignore"`
	Expected []Expectation `yaml:"expected"`
	ignore   []*regexp.Regexp
}

// LoadConfig parses a compare config.
func LoadConfig(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	for _, p := range c.Ignore {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("ignore %q: %w", p, err)
		}
		c.ignore = append(c.ignore, re)
	}
	for i := range c.Expected {
		re, err := regexp.Compile(c.Expected[i].Key)
		if err != nil {
			return nil, fmt.Errorf("expected %q: %w", c.Expected[i].Key, err)
		}
		c.Expected[i].re = re
	}
	return &c, nil
}

func get(f Facts, k string) string {
	if v, ok := f[k]; ok {
		return v
	}
	return absent
}

// Compare classifies every fact that either provisioner changed relative to
// its own baseline (both servers start from the same image).
func Compare(trellisBefore, trellisAfter, convergeBefore, convergeAfter Facts, cfg *Config) []Change {
	if cfg == nil {
		cfg = &Config{}
	}
	keys := map[string]bool{}
	for _, f := range []Facts{trellisBefore, trellisAfter, convergeBefore, convergeAfter} {
		for k := range f {
			keys[k] = true
		}
	}
	var out []Change
	for k := range keys {
		if slices.ContainsFunc(cfg.ignore, func(re *regexp.Regexp) bool { return re.MatchString(k) }) {
			continue
		}
		tb, ta := get(trellisBefore, k), get(trellisAfter, k)
		cb, ca := get(convergeBefore, k), get(convergeAfter, k)
		tChanged, cChanged := ta != tb, ca != cb
		if !tChanged && !cChanged {
			continue
		}
		c := Change{Key: k, Before: tb, Trellis: ta, Converge: ca}
		switch {
		case ta == ca:
			c.Kind = Same
		case tChanged && !cChanged:
			c.Kind = OnlyTrellis
		case !tChanged && cChanged:
			c.Kind = OnlyConverge
		default:
			c.Kind = Differ
		}
		if c.Kind != Same {
			for _, e := range cfg.Expected {
				if e.re.MatchString(k) {
					c.Accepted = e.Reason
					break
				}
			}
		}
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b Change) int { return strings.Compare(a.Key, b.Key) })
	return out
}

func area(key string) string {
	a, _, _ := strings.Cut(key, "/")
	return a
}

func show(v string, max int) string {
	if v == absent {
		return "∅"
	}
	if v == "" {
		return `""`
	}
	v = strings.ReplaceAll(v, "|", "\\|")
	if len(v) > max {
		return v[:max] + "…"
	}
	return v
}

// Report writes a Markdown summary: counts per area, then every unaccepted
// difference, then accepted ones with their reasons.
func Report(w io.Writer, changes []Change) {
	type counts struct{ same, differ, onlyT, onlyC, accepted int }
	by := map[string]*counts{}
	var areas []string
	for _, c := range changes {
		a := area(c.Key)
		if by[a] == nil {
			by[a] = &counts{}
			areas = append(areas, a)
		}
		n := by[a]
		switch {
		case c.Kind == Same:
			n.same++
		case c.Accepted != "":
			n.accepted++
		case c.Kind == Differ:
			n.differ++
		case c.Kind == OnlyTrellis:
			n.onlyT++
		case c.Kind == OnlyConverge:
			n.onlyC++
		}
	}
	slices.Sort(areas)
	fmt.Fprintf(w, "## Summary\n\n| Area | Same | Differ | Only Trellis | Only converge | Accepted |\n|---|---:|---:|---:|---:|---:|\n")
	var tot counts
	for _, a := range areas {
		n := by[a]
		fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d |\n", a, n.same, n.differ, n.onlyT, n.onlyC, n.accepted)
		tot.same += n.same
		tot.differ += n.differ
		tot.onlyT += n.onlyT
		tot.onlyC += n.onlyC
		tot.accepted += n.accepted
	}
	fmt.Fprintf(w, "| **total** | %d | %d | %d | %d | %d |\n", tot.same, tot.differ, tot.onlyT, tot.onlyC, tot.accepted)

	for _, a := range areas {
		var rows []Change
		for _, c := range changes {
			if area(c.Key) == a && c.Kind != Same && c.Accepted == "" {
				rows = append(rows, c)
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n## %s\n\n| Fact | Kind | Trellis | Converge |\n|---|---|---|---|\n", a)
		for _, c := range rows {
			fmt.Fprintf(w, "| `%s` | %s | %s | %s |\n", show(strings.TrimPrefix(c.Key, a+"/"), 140), c.Kind, show(c.Trellis, 90), show(c.Converge, 90))
		}
	}
	var accepted []Change
	for _, c := range changes {
		if c.Accepted != "" {
			accepted = append(accepted, c)
		}
	}
	if len(accepted) > 0 {
		fmt.Fprintf(w, "\n## Accepted differences\n\n| Fact | Kind | Reason |\n|---|---|---|\n")
		for _, c := range accepted {
			fmt.Fprintf(w, "| `%s` | %s | %s |\n", show(c.Key, 120), c.Kind, c.Accepted)
		}
	}
}
