package doctor

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/fatih/color"
)

// ReportSchema is the version of the --json output. Bump it on breaking
// changes.
const ReportSchema = 1

type Report struct {
	Schema       int         `json:"schema"`
	Local        []Result    `json:"local"`
	Environments []EnvReport `json:"environments"`
}

type EnvStatus string

const (
	EnvChecked EnvStatus = "checked"
	EnvSkipped EnvStatus = "skipped"
	EnvError   EnvStatus = "error"
)

type EnvReport struct {
	Name   string       `json:"name"`
	Status EnvStatus    `json:"status"`
	Reason string       `json:"reason,omitempty"`
	Hosts  []HostResult `json:"hosts,omitempty"`
	Fix    *Fix         `json:"fix,omitempty"`
	// FixNote explains why there's no fix when one would otherwise apply.
	FixNote string `json:"fix_note,omitempty"`
}

type HostResult struct {
	Name    string   `json:"name"`
	Address string   `json:"address,omitempty"`
	User    string   `json:"user,omitempty"`
	Error   string   `json:"error,omitempty"`
	Results []Result `json:"results,omitempty"`
}

// ExitCode is 1 if any check failed (or, with strict, warned), else 2 if
// anything couldn't be checked (including when no environment was checked
// at all), else 0.
func (r *Report) ExitCode(strict bool) int {
	failed, incomplete := false, !r.CheckedAny()

	check := func(results []Result) {
		for _, res := range results {
			switch {
			case res.Status == StatusFail || (strict && res.Status == StatusWarn):
				failed = true
			case res.Status == StatusUnknown:
				incomplete = true
			}
		}
	}

	check(r.Local)

	for _, env := range r.Environments {
		if env.Status == EnvError {
			incomplete = true
		}
		for _, host := range env.Hosts {
			if host.Error != "" {
				incomplete = true
			}
			check(host.Results)
		}
	}

	switch {
	case failed:
		return 1
	case incomplete:
		return 2
	default:
		return 0
	}
}

// CheckedAny reports whether at least one environment was checked.
func (r *Report) CheckedAny() bool {
	for _, env := range r.Environments {
		if env.Status == EnvChecked {
			return true
		}
	}
	return false
}

func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

var statusSymbols = map[Status]string{
	StatusOK:      "✓",
	StatusWarn:    "!",
	StatusFail:    "✗",
	StatusUnknown: "?",
	StatusInfo:    "·",
	StatusSkip:    "-",
}

var statusColors = map[Status]*color.Color{
	StatusOK:      color.New(color.FgGreen),
	StatusWarn:    color.New(color.FgYellow),
	StatusFail:    color.New(color.FgRed),
	StatusUnknown: color.New(color.FgMagenta),
	StatusInfo:    color.New(color.FgBlue),
	StatusSkip:    color.New(color.Faint),
}

// notable results are shown in quiet mode.
func notable(r Result) bool {
	return r.Status == StatusFail || r.Status == StatusWarn || r.Status == StatusUnknown
}

// WriteText writes the human-readable report. In quiet mode only problems
// are written, and nothing at all when there are none.
func (r *Report) WriteText(w io.Writer, quiet bool) {
	var sections []string

	if s := renderResults("Local", r.Local, quiet); s != "" {
		sections = append(sections, s)
	}

	for _, env := range r.Environments {
		if s := renderEnv(env, quiet); s != "" {
			sections = append(sections, s)
		}
	}

	if !r.CheckedAny() {
		notice := color.New(color.Bold).Sprint("No servers were checked.")
		// Only suggest naming an environment when they were all skipped;
		// otherwise each one's reason is already shown.
		if !slices.ContainsFunc(r.Environments, func(e EnvReport) bool { return e.Status == EnvError }) {
			notice += " Name an environment to check it, e.g. `trellis doctor production`."
		}
		sections = append(sections, notice+"\n")
	}

	if len(sections) > 0 {
		_, _ = fmt.Fprintln(w, strings.Join(sections, "\n"))
	}
}

// couldntCheck formats an environment or host that couldn't be checked.
func couldntCheck(reason string) string {
	return fmt.Sprintf("  %s couldn't check: %s\n", statusColors[StatusUnknown].Sprint(statusSymbols[StatusUnknown]), indentContinuation(reason, "    "))
}

func renderEnv(env EnvReport, quiet bool) string {
	bold := color.New(color.Bold).SprintFunc()

	switch env.Status {
	case EnvSkipped:
		if quiet {
			return ""
		}
		return fmt.Sprintf("%s %s\n", bold(env.Name), color.New(color.Faint).Sprintf("· skipped: %s", env.Reason))
	case EnvError:
		return bold(env.Name) + "\n" + couldntCheck(env.Reason)
	}

	var b strings.Builder
	multiHost := len(env.Hosts) > 1

	for i, host := range env.Hosts {
		heading := env.Name
		if multiHost || host.Address != "" {
			heading += " · " + hostLabel(host)
		}

		if host.Error != "" {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(bold(heading) + "\n" + couldntCheck(host.Error))
			continue
		}

		body := renderResults("", host.Results, quiet)
		if body == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(bold(heading) + "\n" + body)
	}

	if env.Fix == nil && env.FixNote != "" {
		if b.Len() == 0 {
			b.WriteString(bold(env.Name) + "\n")
		}
		fmt.Fprintf(&b, "\n  %s No upgrade command suggested because %s.\n", bold("Fix:"), env.FixNote)
	}

	if env.Fix != nil {
		if b.Len() == 0 {
			b.WriteString(bold(env.Name) + "\n")
		}
		b.WriteString("\n  " + bold("Fix:") + "\n")
		for _, note := range env.Fix.Notes {
			fmt.Fprintf(&b, "    %s\n", note)
		}
		fmt.Fprintf(&b, "    %s\n", env.Fix.Command)
	}

	return b.String()
}

func hostLabel(h HostResult) string {
	label := h.Name
	if h.Address != "" && h.Address != h.Name {
		label = fmt.Sprintf("%s (%s)", h.Name, h.Address)
	}
	if h.User != "" {
		label = h.User + "@" + label
	}
	return label
}

func renderResults(heading string, results []Result, quiet bool) string {
	var shown []Result
	for _, r := range results {
		if !quiet || notable(r) {
			shown = append(shown, r)
		}
	}

	if len(shown) == 0 {
		return ""
	}

	width := 0
	for _, r := range shown {
		width = max(width, len([]rune(r.Title)))
	}

	var b strings.Builder
	if heading != "" {
		b.WriteString(color.New(color.Bold).Sprint(heading) + "\n")
	}

	pad := strings.Repeat(" ", 4+width+2)
	for _, r := range shown {
		title := r.Title + strings.Repeat(" ", width-len([]rune(r.Title)))
		fmt.Fprintf(&b, "  %s %s  %s\n", statusColors[r.Status].Sprint(statusSymbols[r.Status]), title, r.Message)
		for _, d := range r.Details {
			fmt.Fprintf(&b, "%s%s\n", pad, color.New(color.Faint).Sprint(d))
		}
	}

	return b.String()
}

func indentContinuation(s, indent string) string {
	return strings.ReplaceAll(strings.TrimSpace(s), "\n", "\n"+indent)
}
