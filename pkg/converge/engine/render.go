package engine

import (
	"fmt"
	"io"
	"strings"
)

var opSymbol = map[Op]string{
	OpCreate: "+",
	OpUpdate: "~",
	OpRemove: "-",
	OpRun:    "!",
	OpPrune:  "×",
}

// Render writes a human-readable plan. Sensitive content is never shown;
// only that it changed.
func (p *Plan) Render(w io.Writer, title string) {
	from := "new"
	if p.Generation > 1 {
		from = fmt.Sprint(p.Generation - 1)
	}
	fmt.Fprintf(w, "%s   generation %s → %d   (plan %s)\n", title, from, p.Generation, p.Hash())
	for _, a := range p.Actions {
		line := fmt.Sprintf("  %s %-13s %s", opSymbol[a.Op], a.Resource.Kind, a.Resource.Describe())
		if len(a.Reasons) > 0 {
			line += "   (" + strings.Join(a.Reasons, ", ") + ")"
		}
		_, _ = fmt.Fprintln(w, line)
	}
	if p.Empty() {
		fmt.Fprintf(w, "  ✓ no changes (%d resources up to date)\n", p.Unchanged)
	} else {
		fmt.Fprintf(w, "  ✓ %d unchanged\n", p.Unchanged)
	}
	for _, warn := range p.Warnings {
		fmt.Fprintf(w, "  warning: %s\n", warn)
	}
}

// String renders an apply event for terminals.
func (e Event) String() string {
	s := fmt.Sprintf("  %-8s %s", e.Op, e.Target)
	if e.Message != "" {
		s += "  (" + e.Message + ")"
	}
	return s
}
