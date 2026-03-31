package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

var (
	greenCheck  = color.New(color.FgGreen).Sprint("✓")
	redCross    = color.New(color.FgRed).Sprint("✗")
	faintColor  = color.New(color.Faint)
	yellowColor = color.New(color.FgYellow)
	redColor    = color.New(color.FgRed)
	cyanColor   = color.New(color.FgCyan)
	boldColor   = color.New(color.Bold)
)

// spinnerFrames are braille characters for the spinner animation.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// FailureDetail stores information about a failed task for the end-of-run dump.
type FailureDetail struct {
	RoleName string
	TaskName string
	Host     string
	Msg      string
	Stderr   string
	Stdout   string
}

// RoleState tracks the accumulated state of tasks within a role.
type RoleState struct {
	Name         string
	StartTime    time.Time
	EndTime      time.Time
	Ok           int
	Changed      int
	Skipped      int
	Failed       int
	Unreachable  int
	TotalTasks     int
	SeenTasks      int // actual task starts seen (may exceed TotalTasks for dynamic includes)
	CompletedTasks int
	completedIDs   map[string]bool // track which task IDs have been counted as completed
	ChangedTasks   []changedTask
	CurrentTask    string
}

// markCompleted increments CompletedTasks if this task ID hasn't been counted yet.
// In multi-host mode, multiple runner events fire for the same task ID (one per host).
// We only want to count it once for progress tracking.
func (rs *RoleState) markCompleted(taskID string) {
	if rs.completedIDs[taskID] {
		return
	}
	rs.completedIDs[taskID] = true
	rs.CompletedTasks++
}

// Renderer implements EventHandler and produces pretty terminal output.
type Renderer struct {
	writer    io.Writer
	mu        sync.Mutex
	playName  string
	playStart time.Time

	// Role tracking
	roles      []*RoleState
	currentRole *RoleState
	seenHosts  map[string]bool
	multiHost  bool

	// Standalone tasks (no role prefix)
	standaloneTasks []standaloneTask

	// Global progress tracking
	globalTotal        int
	globalCompleted    int
	globalCompletedIDs map[string]bool
	expectedPerRole    map[string]int // from --list-tasks

	// Failure tracking
	failures []FailureDetail

	// Spinner state
	spinnerActive bool
	spinnerDone   chan struct{}
	spinnerFrame  int
	spinnerLines  int // number of lines currently occupied by spinner output

	// Passthrough mode (after parse error)
	passthrough bool
}

type changedTask struct {
	name string
	host string
}

type standaloneTask struct {
	name     string
	status   string
	duration time.Duration
}

// NewRenderer creates a Renderer that writes to w.
// If taskList is provided, it sets the initial expected task totals for progress tracking.
func NewRenderer(w io.Writer, taskList *TaskList) *Renderer {
	r := &Renderer{
		writer:             w,
		seenHosts:          make(map[string]bool),
		globalCompletedIDs: make(map[string]bool),
	}

	if taskList != nil {
		r.globalTotal = taskList.Total
		r.expectedPerRole = taskList.PerRole
	}

	return r
}

func (r *Renderer) markGlobalCompleted(taskID string) {
	if r.globalCompletedIDs[taskID] {
		return
	}
	r.globalCompletedIDs[taskID] = true
	r.globalCompleted++
}

func (r *Renderer) OnPlayStart(e PlayStartEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.playName = e.Play.Name
	r.playStart = e.Play.Duration.Start

	fmt.Fprintf(r.writer, "\n%s %s\n\n",
		cyanColor.Sprint("▸"),
		boldColor.Sprint(e.Play.Name),
	)
}

func (r *Renderer) OnTaskStart(e TaskStartEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	roleName := e.Task.RoleName()

	if roleName == "" {
		// Standalone tasks: only increment global total if no pre-populated list
		if r.expectedPerRole == nil {
			r.globalTotal++
		}
		return
	}

	if r.currentRole == nil || r.currentRole.Name != roleName {
		// New role starting
		r.finalizeCurrentRole()
		r.startRole(roleName, e.Task.Duration.Start)
	}

	r.currentRole.SeenTasks++
	// If we've exceeded the expected task count (dynamic includes), grow the total
	if r.currentRole.SeenTasks > r.currentRole.TotalTasks {
		r.globalTotal++
		r.currentRole.TotalTasks = r.currentRole.SeenTasks
	}
	r.currentRole.CurrentTask = e.Task.TaskName()
	r.redrawSpinner()
}

func (r *Renderer) OnRunnerOk(e RunnerOkEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.trackHosts(e.Hosts)

	roleName := e.Task.RoleName()
	results := e.HostResults()

	r.markGlobalCompleted(e.Task.ID)

	if roleName == "" {
		// Standalone task
		duration := e.Task.Duration.End.Sub(e.Task.Duration.Start)
		r.standaloneTasks = append(r.standaloneTasks, standaloneTask{
			name:     e.Task.Name,
			status:   "ok",
			duration: duration,
		})
		r.printStandaloneTask(e.Task.Name, "ok", duration)
		return
	}

	role := r.ensureRole(roleName, e.Task.Duration.Start)

	for host, result := range results {
		if result.Changed {
			role.Changed++
			role.ChangedTasks = append(role.ChangedTasks, changedTask{
				name: e.Task.TaskName(),
				host: host,
			})
		} else {
			role.Ok++
		}
	}

	role.markCompleted(e.Task.ID)
	if !e.Task.Duration.End.IsZero() {
		role.EndTime = e.Task.Duration.End
	}
	r.redrawSpinner()
}

func (r *Renderer) OnRunnerSkipped(e RunnerSkippedEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.trackHosts(e.Hosts)

	roleName := e.Task.RoleName()

	r.markGlobalCompleted(e.Task.ID)

	if roleName == "" {
		return // Don't show standalone skipped tasks
	}

	role := r.ensureRole(roleName, e.Task.Duration.Start)
	role.Skipped++
	role.markCompleted(e.Task.ID)
	if !e.Task.Duration.End.IsZero() {
		role.EndTime = e.Task.Duration.End
	}
	r.redrawSpinner()
}

func (r *Renderer) OnRunnerFailed(e RunnerFailedEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.trackHosts(e.Hosts)

	roleName := e.Task.RoleName()
	results := e.HostResults()

	// Record failures regardless of whether task has a role
	for host, result := range results {
		r.failures = append(r.failures, FailureDetail{
			RoleName: roleName,
			TaskName: e.Task.TaskName(),
			Host:     host,
			Msg:      result.Msg,
			Stderr:   result.Stderr,
			Stdout:   result.Stdout,
		})
	}

	r.markGlobalCompleted(e.Task.ID)

	if roleName == "" {
		// Standalone failed task — render inline
		duration := e.Task.Duration.End.Sub(e.Task.Duration.Start)
		r.printStandaloneTask(e.Task.Name, "failed", duration)
		r.printStandaloneFailures(e.Task.Name, results)
		return
	}

	role := r.ensureRole(roleName, e.Task.Duration.Start)
	for range results {
		role.Failed++
	}

	role.markCompleted(e.Task.ID)
	if !e.Task.Duration.End.IsZero() {
		role.EndTime = e.Task.Duration.End
	}
	r.redrawSpinner()
}

func (r *Renderer) OnRunnerUnreachable(e RunnerUnreachableEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.trackHosts(e.Hosts)

	roleName := e.Task.RoleName()
	results := e.HostResults()

	// Record failures regardless of whether task has a role
	for host, result := range results {
		r.failures = append(r.failures, FailureDetail{
			RoleName: roleName,
			TaskName: e.Task.TaskName(),
			Host:     host,
			Msg:      result.Msg,
		})
	}

	r.markGlobalCompleted(e.Task.ID)

	if roleName == "" {
		// Standalone unreachable task — render inline
		duration := e.Task.Duration.End.Sub(e.Task.Duration.Start)
		r.printStandaloneTask(e.Task.Name, "failed", duration)
		r.printStandaloneFailures(e.Task.Name, results)
		return
	}

	role := r.ensureRole(roleName, e.Task.Duration.Start)
	for range results {
		role.Unreachable++
	}

	role.markCompleted(e.Task.ID)
	r.redrawSpinner()
}

func (r *Renderer) OnStats(e StatsEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.finalizeCurrentRole()
	r.stopSpinner()

	// Summary line
	totalOk := 0
	totalChanged := 0
	totalSkipped := 0
	totalFailed := 0
	totalUnreachable := 0

	for _, stats := range e.Stats {
		totalOk += stats.Ok
		totalChanged += stats.Changed
		totalSkipped += stats.Skipped
		totalFailed += stats.Failures
		totalUnreachable += stats.Unreachable
	}

	totalDuration := time.Since(r.playStart)
	if !r.playStart.IsZero() {
		totalDuration = e.Timestamp.Sub(r.playStart)
	}

	fmt.Fprintf(r.writer, "  %s\n", faintColor.Sprint(strings.Repeat("─", 50)))

	parts := []string{}
	parts = append(parts, fmt.Sprintf("%d ok", totalOk))
	if totalChanged > 0 {
		parts = append(parts, yellowColor.Sprintf("%d changed", totalChanged))
	}
	if totalSkipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", totalSkipped))
	}
	if totalFailed > 0 {
		parts = append(parts, redColor.Sprintf("%d failed", totalFailed))
	}
	if totalUnreachable > 0 {
		parts = append(parts, redColor.Sprintf("%d unreachable", totalUnreachable))
	}

	fmt.Fprintf(r.writer, "  Done: %s %s\n",
		strings.Join(parts, faintColor.Sprint(" · ")),
		faintColor.Sprintf("%s", formatDuration(totalDuration)),
	)

	// Multi-host stats
	if r.multiHost && len(e.Stats) > 1 {
		fmt.Fprintln(r.writer)
		for host, stats := range e.Stats {
			hostParts := []string{}
			hostParts = append(hostParts, fmt.Sprintf("%d ok", stats.Ok))
			if stats.Changed > 0 {
				hostParts = append(hostParts, yellowColor.Sprintf("%d changed", stats.Changed))
			}
			if stats.Failures > 0 {
				hostParts = append(hostParts, redColor.Sprintf("%d failed", stats.Failures))
			}
			fmt.Fprintf(r.writer, "    %s: %s\n", host, strings.Join(hostParts, faintColor.Sprint(" · ")))
		}
	}

	// Failure details at end
	if len(r.failures) > 0 {
		fmt.Fprintf(r.writer, "\n  %s\n\n", redColor.Sprint("─── Failures "+strings.Repeat("─", 37)))

		for _, f := range r.failures {
			taskDisplay := f.TaskName
			if f.RoleName != "" {
				taskDisplay = f.RoleName + " : " + f.TaskName
			}

			hostSuffix := ""
			if r.multiHost {
				hostSuffix = fmt.Sprintf(" (host: %s)", f.Host)
			}

			fmt.Fprintf(r.writer, "  %s %s%s\n",
				redColor.Sprint("FAILED:"),
				taskDisplay,
				faintColor.Sprint(hostSuffix),
			)

			if f.Msg != "" {
				fmt.Fprintf(r.writer, "  msg: %s\n", f.Msg)
			}
			if f.Stderr != "" {
				fmt.Fprintf(r.writer, "  stderr: %s\n", f.Stderr)
			}
			if f.Stdout != "" {
				fmt.Fprintf(r.writer, "  stdout: %s\n", f.Stdout)
			}
			fmt.Fprintln(r.writer)
		}
	}

	fmt.Fprintln(r.writer)
}

func (r *Renderer) OnParseError(line string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.passthrough = true
	r.stopSpinner()
	r.finalizeCurrentRole()

	// Write the unparseable line through
	fmt.Fprintln(r.writer, line)
}

// trackHosts updates the set of known hosts and detects multi-host mode.
func (r *Renderer) trackHosts(hosts interface{}) {
	var hostNames []string

	switch h := hosts.(type) {
	case map[string]json.RawMessage:
		for name := range h {
			hostNames = append(hostNames, name)
		}
	}

	for _, name := range hostNames {
		r.seenHosts[name] = true
	}

	if len(r.seenHosts) > 1 {
		r.multiHost = true
	}
}

func (r *Renderer) startRole(name string, startTime time.Time) {
	role := &RoleState{
		Name:         name,
		StartTime:    startTime,
		completedIDs: make(map[string]bool),
	}

	// Pre-populate expected task count from --list-tasks if available
	if r.expectedPerRole != nil {
		if count, ok := r.expectedPerRole[name]; ok {
			role.TotalTasks = count
		}
	}

	r.roles = append(r.roles, role)
	r.currentRole = role
	r.startSpinner()
}

func (r *Renderer) ensureRole(name string, startTime time.Time) *RoleState {
	if r.currentRole != nil && r.currentRole.Name == name {
		return r.currentRole
	}

	// Look for existing role (shouldn't normally happen, but be safe)
	for _, role := range r.roles {
		if role.Name == name {
			return role
		}
	}

	// Create new role
	r.finalizeCurrentRole()
	r.startRole(name, startTime)
	return r.currentRole
}

func (r *Renderer) finalizeCurrentRole() {
	if r.currentRole == nil {
		return
	}

	r.stopSpinner()
	r.clearSpinnerLines()
	r.printRoleSummary(r.currentRole)
	r.currentRole = nil
}

func (r *Renderer) printStandaloneTask(name string, status string, duration time.Duration) {
	icon := greenCheck
	if status == "failed" {
		icon = redCross
	}

	durStr := fmt.Sprintf("%7s", formatDuration(duration))
	rightPart := faintColor.Sprint(durStr)

	prefix := fmt.Sprintf("  %s %s ", icon, name)
	prefixLen := visibleLen(prefix)
	rightLen := visibleLen(rightPart)

	const lineWidth = 72
	dotsNeeded := lineWidth - prefixLen - rightLen
	if dotsNeeded < 2 {
		dotsNeeded = 2
	}
	dots := faintColor.Sprint(" " + strings.Repeat("·", dotsNeeded-1))

	fmt.Fprintf(r.writer, "%s%s %s\n", prefix, dots, rightPart)
}

func (r *Renderer) printStandaloneFailures(taskName string, results map[string]RunnerFailResult) {
	for host, result := range results {
		msg := result.Msg
		if len(msg) > 100 {
			msg = msg[:100] + "..."
		}

		hostSuffix := ""
		if r.multiHost || len(results) > 1 {
			hostSuffix = fmt.Sprintf(" (%s)", host)
		}

		fmt.Fprintf(r.writer, "    %s %s\n",
			faintColor.Sprint("↳"),
			redColor.Sprintf("FAILED: %s%s", taskName, hostSuffix),
		)
		if msg != "" {
			fmt.Fprintf(r.writer, "      %s\n", msg)
		}
	}
}

func (r *Renderer) printRoleSummary(role *RoleState) {
	hasFailed := role.Failed > 0 || role.Unreachable > 0

	icon := greenCheck
	if hasFailed {
		icon = redCross
	}

	// Fixed-width status columns: always show all fields for consistency
	sep := faintColor.Sprint("  ")

	totalOk := role.Ok + role.Changed
	okStr := fmt.Sprintf("%2d ok", totalOk)

	var changedStr string
	if role.Changed > 0 {
		changedStr = yellowColor.Sprintf("%2d changed", role.Changed)
	} else {
		changedStr = faintColor.Sprintf("%2d changed", 0)
	}

	skippedStr := faintColor.Sprintf("%2d skipped", role.Skipped)

	failedTotal := role.Failed + role.Unreachable
	var failedStr string
	if failedTotal > 0 {
		failedStr = redColor.Sprintf("%2d failed", failedTotal)
	} else {
		failedStr = faintColor.Sprintf("%2d failed", 0)
	}

	statusStr := okStr + sep + changedStr + sep + skippedStr + sep + failedStr

	duration := role.EndTime.Sub(role.StartTime)
	durStr := fmt.Sprintf("%7s", formatDuration(duration))

	// Build: "  ✓ rolename ···· status   duration"
	prefix := fmt.Sprintf("  %s %s ", icon, role.Name)
	prefixLen := visibleLen(prefix)
	statusLen := visibleLen(statusStr)
	durLen := len(durStr)

	const lineWidth = 72
	rightPartLen := statusLen + 1 + durLen // status + space + duration
	dotsNeeded := lineWidth - prefixLen - rightPartLen
	if dotsNeeded < 2 {
		dotsNeeded = 2
	}
	dots := faintColor.Sprint(" " + strings.Repeat("·", dotsNeeded-1))

	fmt.Fprintf(r.writer, "%s%s %s %s\n",
		prefix,
		dots,
		statusStr,
		faintColor.Sprint(durStr),
	)

	// Show changed task names
	for _, ct := range role.ChangedTasks {
		display := ct.name
		if r.multiHost {
			display = fmt.Sprintf("%s (%s)", ct.name, ct.host)
		}
		fmt.Fprintf(r.writer, "    %s %s\n",
			faintColor.Sprint("↳"),
			yellowColor.Sprintf("changed: %s", display),
		)
	}

	// Show inline failure summaries
	for _, f := range r.failures {
		if f.RoleName == role.Name {
			msg := f.Msg
			if len(msg) > 100 {
				msg = msg[:100] + "..."
			}
			fmt.Fprintf(r.writer, "    %s %s\n",
				faintColor.Sprint("↳"),
				redColor.Sprintf("FAILED: %s", f.TaskName),
			)
			if msg != "" {
				fmt.Fprintf(r.writer, "      %s\n", msg)
			}
		}
	}
}

// termWidth returns the current terminal width, defaulting to 80.
func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// Spinner methods

func (r *Renderer) startSpinner() {
	r.stopSpinner()
	r.spinnerActive = true
	r.spinnerDone = make(chan struct{})
	r.spinnerFrame = 0

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-r.spinnerDone:
				return
			case <-ticker.C:
				r.mu.Lock()
				if r.spinnerActive && r.currentRole != nil {
					r.spinnerFrame = (r.spinnerFrame + 1) % len(spinnerFrames)
					r.drawSpinnerLine()
				}
				r.mu.Unlock()
			}
		}
	}()
}

func (r *Renderer) stopSpinner() {
	if r.spinnerActive {
		r.spinnerActive = false
		if r.spinnerDone != nil {
			close(r.spinnerDone)
			r.spinnerDone = nil
		}
	}
}

func (r *Renderer) redrawSpinner() {
	if r.spinnerActive && r.currentRole != nil {
		r.drawSpinnerLine()
	}
}

func (r *Renderer) clearSpinnerLines() {
	for i := 0; i < r.spinnerLines; i++ {
		if i > 0 {
			fmt.Fprint(r.writer, "\033[A")
		}
		fmt.Fprint(r.writer, "\r\033[K")
	}
	r.spinnerLines = 0
}

func (r *Renderer) drawSpinnerLine() {
	if r.currentRole == nil {
		return
	}

	r.clearSpinnerLines()

	role := r.currentRole
	frame := cyanColor.Sprint(spinnerFrames[r.spinnerFrame])
	width := termWidth()
	lines := 0

	// Line 1: full-width progress bar with task counter
	if r.globalTotal > 0 {
		counter := fmt.Sprintf(" %d/%d", r.globalCompleted, r.globalTotal)
		barWidth := width - 4 - len(counter) // 4 = 2 indent + 2 padding
		if barWidth < 10 {
			barWidth = 10
		}

		completed := r.globalCompleted
		if completed > r.globalTotal {
			completed = r.globalTotal
		}
		filled := (completed * barWidth) / r.globalTotal
		bar := strings.Repeat("━", filled) + strings.Repeat("─", barWidth-filled)

		fmt.Fprintf(r.writer, "  %s%s",
			faintColor.Sprint(bar),
			faintColor.Sprint(counter),
		)
		lines++
	}

	// Line 2: spinner + role name + current task
	taskInfo := ""
	if role.CurrentTask != "" {
		taskInfo = fmt.Sprintf(" %s %s",
			faintColor.Sprint("↳"),
			faintColor.Sprint(role.CurrentTask),
		)
	}

	if lines > 0 {
		fmt.Fprint(r.writer, "\n")
	}
	fmt.Fprintf(r.writer, "  %s %s%s", frame, role.Name, taskInfo)
	lines++

	r.spinnerLines = lines
}

// visibleLen returns the display width of a string, stripping ANSI escape codes.
func visibleLen(s string) int {
	return len(ansiRegex.ReplaceAllString(s, ""))
}

// formatDuration formats a duration in a human-friendly way.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}

	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}

	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}

	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%dm %ds", minutes, seconds)
}
