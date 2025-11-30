package output

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"
)

type Formatter struct {
	currentTaskName            string
	taskStartTime              time.Time
	lastRole                   string
	progressbar                *pterm.ProgressbarPrinter
	currentTaskIndentation     string
	roleTaskSummary            map[string]int
	roleHeaderLineCount        int
	roleStartTime              time.Time
	currentTaskHostCount       int
	hostsInRole                map[string]struct{}
	disableClearing            bool
	permanentlyDisableClearing bool
	tasksStartedCount          int
}

var roleRegex = regexp.MustCompile(`roles/([^/]+)/`)

var symbols = map[string]string{
	"success": "✓",
	"failed":  "✗",
	"changed": "~",
	"skipped": "→",
}

func (f *Formatter) Process(reader io.Reader, totalTasks int) {
	pterm.Println() // Blank line
	f.progressbar, _ = pterm.DefaultProgressbar.WithTotal(totalTasks + 1).WithRemoveWhenDone(true).Start()

	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			pterm.Error.Printf(
				"Trellis CLI Internal Error: Failed to parse an Ansible event.\n"+
					"This is a CLI issue, not an error from the Ansible playbook.\n"+
					"Please report this bug at https://github.com/roots/trellis-cli/issues\n"+
					"Raw Event Line: %s\n"+
					"Error: %v\n",
				line,
				err,
			)
			f.permanentlyDisableClearing = true
			continue
		}

		switch event.Event {
		case "v2_playbook_on_play_start":
			f.handlePlayStart(line)
		case "v2_playbook_on_task_start":
			f.handleTaskStart(line)
		case "v2_runner_on_ok":
			f.handleRunnerOk(line)
		case "v2_runner_on_failed":
			f.handleRunnerFailed(line)
		case "v2_runner_on_skipped":
			f.handleRunnerSkipped(line)
		case "v2_playbook_on_stats":
			f.handleStats(line)
		default:
			// Ignore other events for now
		}
	}

	if err := scanner.Err(); err != nil {
		if err != io.EOF {
			pterm.Error.Println("Error reading from scanner:", err)
		}
	}

	f.progressbar.Stop()
}

func (f *Formatter) handlePlayStart(line string) {
	var playStartEvent PlaybookOnPlayStartEvent
	if err := json.Unmarshal([]byte(line), &playStartEvent); err != nil {
		pterm.Error.Println("Error unmarshalling play start event:", err)
		return
	}

	pterm.DefaultSection.WithLevel(1).Println(fmt.Sprintf("PLAY [%s]", playStartEvent.Play.Name))
}

func (f *Formatter) handleTaskStart(line string) {
	var taskStartEvent PlaybookOnTaskStartEvent
	if err := json.Unmarshal([]byte(line), &taskStartEvent); err != nil {
		pterm.Error.Println("Error unmarshalling task start event:", err)
		return
	}

	f.currentTaskName = taskStartEvent.Task.Name
	f.taskStartTime = time.Now()
	f.currentTaskHostCount = len(taskStartEvent.Hosts)

	// Hybrid role detection: First, try to parse the logical role from the task name.
	// If that fails (eg, for tasks like "Gathering Facts"), fall back to the physical
	// role based on the task's file path.
	var role string
	parts := strings.SplitN(f.currentTaskName, " : ", 2)
	if len(parts) > 1 {
		role = parts[0]
	} else {
		matches := roleRegex.FindStringSubmatch(taskStartEvent.Task.Path)
		if len(matches) > 1 {
			role = matches[1]
		} else {
			role = ""
		}
	}

	if role != f.lastRole {
		if f.lastRole != "" {
			f.summarizeCompletedRole()
		}
		f.lastRole = role
		f.roleTaskSummary = make(map[string]int)
		f.roleHeaderLineCount = 0
		f.roleStartTime = time.Now()
		f.hostsInRole = make(map[string]struct{})

		if role != "" {
			pterm.Printf("◉ %s\n", role)
			f.roleHeaderLineCount++
		}
	}

	for host := range taskStartEvent.Hosts {
		f.hostsInRole[host] = struct{}{}
	}

	// Remove role prefix from task name if present
	rolePrefix := role + " : "
	if strings.HasPrefix(f.currentTaskName, rolePrefix) {
		f.currentTaskName = strings.TrimPrefix(f.currentTaskName, rolePrefix)
	}

	f.tasksStartedCount++
	if f.tasksStartedCount >= f.progressbar.Total {
		f.progressbar.Total = f.tasksStartedCount + 1
	}

	if role != "" {
		f.currentTaskIndentation = "  "
	} else {
		f.currentTaskIndentation = ""
	}

	f.roleHeaderLineCount++
	pterm.Printf("%s%s %s\n", f.currentTaskIndentation, pterm.Gray("●"), f.currentTaskName)
}

func (f *Formatter) handleRunnerOk(line string) {
	pterm.Print("\033[1A")
	pterm.Print("\033[2K\r")

	var okEvent RunnerOnOkEvent
	if err := json.Unmarshal([]byte(line), &okEvent); err != nil {
		pterm.Error.Println("Error unmarshalling runner ok event:", err)
		return
	}

	isChanged := false
	individualStatuses := make(map[string]string)
	for host, result := range okEvent.Hosts {
		if result.Changed {
			isChanged = true
			individualStatuses[host] = "changed"
		} else {
			individualStatuses[host] = "ok"
		}
	}

	if isChanged {
		f.printTaskLine(symbols["changed"], "CHANGED", pterm.FgYellow)
		f.roleTaskSummary["changed"]++
	} else {
		f.printTaskLine(symbols["success"], "OK", pterm.FgGreen)
		f.roleTaskSummary["ok"]++
	}

	if f.currentTaskHostCount > 1 {
		for host, status := range individualStatuses {
			var statusText string
			switch status {
			case "changed":
				statusText = pterm.FgYellow.Sprint("CHANGED")
			case "ok":
				statusText = pterm.FgGreen.Sprint("OK")
			}
			pterm.Printf("%s  - %s: %s\n", f.currentTaskIndentation, host, statusText)
			f.roleHeaderLineCount++
		}
	}

	f.progressbar.Increment()
}

func (f *Formatter) handleRunnerFailed(line string) {
	pterm.Print("\033[1A")
	pterm.Print("\033[2K\r")

	var failedEvent RunnerOnFailedEvent
	if err := json.Unmarshal([]byte(line), &failedEvent); err != nil {
		pterm.Error.Println("Error unmarshalling runner failed event:", err)
		return
	}

	// Aggregated status is always "failed".
	f.printTaskLine(symbols["failed"], "FAILED", pterm.FgRed)
	f.roleTaskSummary["failed"]++

	if f.currentTaskHostCount > 1 {
		// Multi-host failure
		for host, result := range failedEvent.Hosts {
			pterm.Printf("%s  - %s: %s\n", f.currentTaskIndentation, host, pterm.FgRed.Sprint("FAILED"))
			f.roleHeaderLineCount++

			var errorDetails strings.Builder
			errorDetails.WriteString(fmt.Sprintf("%s    Error: %s", f.currentTaskIndentation, result.Msg))

			if result.Stdout != "" {
				errorDetails.WriteString(fmt.Sprintf("\n%s    Stdout: %s", f.currentTaskIndentation, result.Stdout))
			}
			if result.Stderr != "" {
				errorDetails.WriteString(fmt.Sprintf("\n%s    Stderr: %s", f.currentTaskIndentation, result.Stderr))
			}

			errorMessage := errorDetails.String()
			pterm.Error.Println(errorMessage)
			f.roleHeaderLineCount += strings.Count(errorMessage, "\n") + 1
		}
	} else {
		// Single-host failure
		for _, result := range failedEvent.Hosts {
			var errorDetails strings.Builder
			errorDetails.WriteString(fmt.Sprintf("  Error: %s", result.Msg))

			if result.Stdout != "" {
				errorDetails.WriteString(fmt.Sprintf("\n  Stdout: %s", result.Stdout))
			}
			if result.Stderr != "" {
				errorDetails.WriteString(fmt.Sprintf("\n  Stderr: %s", result.Stderr))
			}

			errorMessage := errorDetails.String()
			pterm.Error.Println(errorMessage)
			f.roleHeaderLineCount += strings.Count(errorMessage, "\n") + 1
			break // Should only be one host
		}
	}

	f.progressbar.Increment()
}

func (f *Formatter) handleRunnerSkipped(line string) {
	pterm.Print("\033[1A")
	pterm.Print("\033[2K\r")

	var skippedEvent RunnerOnSkippedEvent
	if err := json.Unmarshal([]byte(line), &skippedEvent); err != nil {
		pterm.Error.Println("Error unmarshalling runner skipped event:", err)
		return
	}

	f.printTaskLine(symbols["skipped"], "SKIPPED", pterm.FgCyan)
	f.roleTaskSummary["skipped"]++

	if f.currentTaskHostCount > 1 {
		for host := range skippedEvent.Hosts {
			pterm.Printf("%s  - %s: %s\n", f.currentTaskIndentation, host, pterm.FgCyan.Sprint("SKIPPED"))
			f.roleHeaderLineCount++
		}
	}

	f.progressbar.Increment()
}

func (f *Formatter) handleStats(line string) {
	var statsEvent PlaybookOnStatsEvent
	if err := json.Unmarshal([]byte(line), &statsEvent); err != nil {
		pterm.Error.Println("Error unmarshalling stats event:", err)
		return
	}

	if f.lastRole != "" {
		f.summarizeCompletedRole()
	}

	var recapBuilder strings.Builder

	for host, stats := range statsEvent.Stats {
		summary := []string{}
		if stats.Ok > 0 {
			summary = append(summary, pterm.Green(fmt.Sprintf("%s ok=%d", symbols["success"], stats.Ok)))
		}
		if stats.Changed > 0 {
			summary = append(summary, pterm.Yellow(fmt.Sprintf("%s changed=%d", symbols["changed"], stats.Changed)))
		}
		if stats.Skipped > 0 {
			summary = append(summary, pterm.Cyan(fmt.Sprintf("%s skipped=%d", symbols["skipped"], stats.Skipped)))
		}
		if stats.Failures > 0 {
			summary = append(summary, pterm.Red(fmt.Sprintf("%s failed=%d", symbols["failed"], stats.Failures)))
		}
		if stats.Unreachable > 0 {
			summary = append(summary, pterm.Red(fmt.Sprintf("%s unreachable=%d", symbols["failed"], stats.Unreachable)))
		}

		paddedHost := fmt.Sprintf("%-20s", host)
		recapBuilder.WriteString(fmt.Sprintf("%s : %s\n", paddedHost, strings.Join(summary, "  ")))
	}

	pterm.Println()
	pterm.DefaultBox.WithTitle("Summary").Println(recapBuilder.String())
}

func (f *Formatter) printTaskLine(symbol, status string, statusColor pterm.Color) {
	width := pterm.GetTerminalWidth()
	duration := time.Since(f.taskStartTime)
	timeStr := fmt.Sprintf("%dms", duration.Milliseconds())
	timeStr = fmt.Sprintf("%8s", timeStr)

	coloredSymbol := statusColor.Sprint(symbol)
	coloredStatusStr := pterm.Gray(fmt.Sprintf("(%s)", status))

	leftStr := fmt.Sprintf("%s%s %s %s", f.currentTaskIndentation, coloredSymbol, f.currentTaskName, coloredStatusStr)

	uncoloredLeftStrWidth := runewidth.StringWidth(f.currentTaskIndentation) +
		runewidth.StringWidth(symbol) + 1 +
		runewidth.StringWidth(f.currentTaskName) + 1 +
		runewidth.StringWidth(fmt.Sprintf("(%s)", status)) // Use uncolored status for width calculation

	padding := width - uncoloredLeftStrWidth - len(timeStr) - 2

	if padding < 0 {
		padding = 0
	}

	dots := strings.Repeat(".", padding)

	pterm.Printf("%s %s %s\n", leftStr, pterm.Gray(dots), pterm.Gray(timeStr))
}

func Process(reader io.Reader, totalTasks int) {
	formatter := &Formatter{
		roleTaskSummary: make(map[string]int),
		disableClearing: os.Getenv("TRELLIS_CLI_VERBOSE_OUTPUT") != "",
	}
	formatter.Process(reader, totalTasks)
}

func (f *Formatter) summarizeCompletedRole() {
	if f.disableClearing || f.permanentlyDisableClearing {
		return
	}

	if f.lastRole == "" {
		return
	}

	// 1. Clear previous lines
	for i := 0; i < f.roleHeaderLineCount; i++ {
		pterm.Print("\033[1A")   // Move cursor up
		pterm.Print("\033[2K\r") // Clear line and move to beginning
	}

	// 2. Calculate summary data
	totalTasks := 0
	for _, count := range f.roleTaskSummary {
		totalTasks += count
	}

	duration := time.Since(f.roleStartTime)

	statusSymbol := symbols["success"]
	statusColor := pterm.FgGreen
	if f.roleTaskSummary["failed"] > 0 {
		statusSymbol = symbols["failed"]
		statusColor = pterm.FgRed
	}

	// 3. Format and print the summary line
	width := pterm.GetTerminalWidth()
	timeStr := fmt.Sprintf("%dms", duration.Milliseconds())
	timeStr = fmt.Sprintf("%8s", timeStr)

	roleName := f.lastRole
	coloredTaskCountStr := pterm.Gray(fmt.Sprintf("(%d tasks, %d hosts)", totalTasks, len(f.hostsInRole)))

	coloredSymbol := statusColor.Sprint(statusSymbol)

	leftStr := fmt.Sprintf("%s %s %s", coloredSymbol, roleName, coloredTaskCountStr)

	// Correct padding calculation
	uncoloredLeftStrWidth := runewidth.StringWidth(statusSymbol) + 1 + runewidth.StringWidth(roleName) + 1 + runewidth.StringWidth(fmt.Sprintf("(%d tasks, %d hosts)", totalTasks, len(f.hostsInRole)))

	padding := width - uncoloredLeftStrWidth - len(timeStr) - 2
	if padding < 0 {
		padding = 0
	}
	dots := strings.Repeat(".", padding)

	pterm.Printf("%s %s %s\n", leftStr, pterm.Gray(dots), pterm.Gray(timeStr))
}
