package doctor

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/fatih/color"
)

type Report struct {
	Local        []Result
	Environments []EnvReport
}

type EnvStatus string

const (
	EnvChecked EnvStatus = "checked"
	EnvSkipped EnvStatus = "skipped"
	EnvError   EnvStatus = "error"
)

type EnvReport struct {
	Name   string
	Status EnvStatus
	Reason string
	Hosts  []HostResult
	Fix    *Fix
	// FixNote explains why there's no fix when one would otherwise apply.
	FixNote string
}

type HostResult struct {
	Name    string
	Address string
	User    string
	// Error is set when the host couldn't be checked.
	Error   string
	Results []Result

	// upgrades are services that `trellis provision` can bring up to date.
	upgrades []*Service
	// withheld are services that provisioning with the package state set to
	// latest could switch to another version on this host (or that can't be
	// judged), so no upgrade command may include them.
	withheld []*Service
	// staleLists is true when the apt package lists are too old to confirm
	// packages are current; such packages are included in upgrades, since
	// provisioning refreshes the lists first.
	staleLists bool
}

// ExitCode is 1 if any check failed, else 2 if anything couldn't be checked
// (including when no environment was checked at all), else 0.
func (r *Report) ExitCode() int {
	results := slices.Clone(r.Local)
	incomplete := !r.checkedAny()

	for _, env := range r.Environments {
		incomplete = incomplete || env.Status == EnvError
		for _, host := range env.Hosts {
			incomplete = incomplete || host.Error != ""
			results = append(results, host.Results...)
		}
	}

	switch {
	case slices.ContainsFunc(results, func(res Result) bool { return res.Status == StatusFail }):
		return 1
	case incomplete || slices.ContainsFunc(results, func(res Result) bool { return res.Status == StatusUnknown }):
		return 2
	default:
		return 0
	}
}

func (r *Report) checkedAny() bool {
	return slices.ContainsFunc(r.Environments, func(e EnvReport) bool { return e.Status == EnvChecked })
}

var statusStyles = map[Status]struct {
	symbol string
	color  *color.Color
}{
	StatusOK:      {"✓", color.New(color.FgGreen)},
	StatusWarn:    {"!", color.New(color.FgYellow)},
	StatusFail:    {"✗", color.New(color.FgRed)},
	StatusUnknown: {"?", color.New(color.FgMagenta)},
	StatusInfo:    {"·", color.New(color.FgBlue)},
	StatusSkip:    {"-", color.New(color.Faint)},
}

var bold = color.New(color.Bold).SprintFunc()

func symbol(s Status) string {
	return statusStyles[s].color.Sprint(statusStyles[s].symbol)
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

	if !r.checkedAny() {
		notice := bold("No servers were checked.")
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
	return fmt.Sprintf("  %s couldn't check: %s\n", symbol(StatusUnknown), strings.ReplaceAll(strings.TrimSpace(reason), "\n", "\n    "))
}

func renderEnv(env EnvReport, quiet bool) string {
	switch env.Status {
	case EnvSkipped:
		if quiet {
			return ""
		}
		return fmt.Sprintf("%s %s\n", bold(env.Name), color.New(color.Faint).Sprintf("· skipped: %s", env.Reason))
	case EnvError:
		return bold(env.Name) + "\n" + couldntCheck(env.Reason)
	}

	var sections []string

	for _, host := range env.Hosts {
		heading := env.Name
		if len(env.Hosts) > 1 || host.Address != "" {
			heading += " · " + hostLabel(host)
		}

		if host.Error != "" {
			sections = append(sections, bold(heading)+"\n"+couldntCheck(host.Error))
		} else if body := renderResults("", host.Results, quiet); body != "" {
			sections = append(sections, bold(heading)+"\n"+body)
		}
	}

	if env.Fix != nil || env.FixNote != "" {
		if len(sections) == 0 {
			sections = append(sections, bold(env.Name)+"\n")
		}

		var fix strings.Builder
		fix.WriteString("  " + bold("Fix:"))
		if env.Fix == nil {
			fmt.Fprintf(&fix, " No upgrade command suggested because %s.\n", env.FixNote)
		} else {
			fix.WriteString("\n")
			for _, line := range append(slices.Clone(env.Fix.Notes), env.Fix.Command) {
				fmt.Fprintf(&fix, "    %s\n", line)
			}
		}
		sections = append(sections, fix.String())
	}

	return strings.Join(sections, "\n")
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
	// Quiet mode shows only problems.
	shown := slices.DeleteFunc(slices.Clone(results), func(r Result) bool {
		return quiet && r.Status != StatusFail && r.Status != StatusWarn && r.Status != StatusUnknown
	})

	if len(shown) == 0 {
		return ""
	}

	width := 0
	for _, r := range shown {
		width = max(width, len([]rune(r.Title)))
	}

	var b strings.Builder
	if heading != "" {
		b.WriteString(bold(heading) + "\n")
	}

	pad := strings.Repeat(" ", 4+width+2)
	for _, r := range shown {
		title := r.Title + strings.Repeat(" ", width-len([]rune(r.Title)))
		fmt.Fprintf(&b, "  %s %s  %s\n", symbol(r.Status), title, r.Message)
		for _, d := range r.Details {
			fmt.Fprintf(&b, "%s%s\n", pad, color.New(color.Faint).Sprint(d))
		}
	}

	return b.String()
}
