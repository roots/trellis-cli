package output

import (
	"encoding/json"
	"strings"
	"time"
)

// Event types from Ansible's ansible.posix.jsonl callback plugin.
const (
	EventPlayStart       = "v2_playbook_on_play_start"
	EventTaskStart       = "v2_playbook_on_task_start"
	EventRunnerOk        = "v2_runner_on_ok"
	EventRunnerSkipped   = "v2_runner_on_skipped"
	EventRunnerFailed    = "v2_runner_on_failed"
	EventRunnerUnreachable = "v2_runner_on_unreachable"
	EventStats           = "v2_playbook_on_stats"
)

// EventEnvelope is the minimal structure shared by all JSONL events.
type EventEnvelope struct {
	EventType string    `json:"_event"`
	Timestamp time.Time `json:"_timestamp"`
}

// TaskInfo contains task metadata present in task_start and runner events.
type TaskInfo struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Path     string       `json:"path"`
	Duration TaskDuration `json:"duration"`
}

// TaskDuration tracks the start and end times of a task.
type TaskDuration struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// RoleName extracts the role name from a task name.
// Ansible task names follow the pattern "role : task name".
// Returns empty string if there is no role prefix.
func (t TaskInfo) RoleName() string {
	if idx := strings.Index(t.Name, " : "); idx != -1 {
		return t.Name[:idx]
	}
	return ""
}

// TaskName extracts just the task name, stripping the role prefix if present.
func (t TaskInfo) TaskName() string {
	if idx := strings.Index(t.Name, " : "); idx != -1 {
		return t.Name[idx+3:]
	}
	return t.Name
}

// PlayStartEvent is emitted when a play begins.
type PlayStartEvent struct {
	EventEnvelope
	Play struct {
		Name     string `json:"name"`
		ID       string `json:"id"`
		Duration struct {
			Start time.Time `json:"start"`
		} `json:"duration"`
		Path string `json:"path"`
	} `json:"play"`
}

// TaskStartEvent is emitted when a task begins execution.
type TaskStartEvent struct {
	EventEnvelope
	Task TaskInfo `json:"task"`
}

// RunnerResult contains per-host result data for ok/skipped events.
type RunnerResult struct {
	Changed bool   `json:"changed"`
	Action  string `json:"action"`
}

// RunnerOkEvent is emitted when a task succeeds on a host.
type RunnerOkEvent struct {
	EventEnvelope
	Hosts map[string]json.RawMessage `json:"hosts"`
	Task  TaskInfo                   `json:"task"`
}

// HostResult extracts the RunnerResult for each host.
func (e RunnerOkEvent) HostResults() map[string]RunnerResult {
	results := make(map[string]RunnerResult, len(e.Hosts))
	for host, raw := range e.Hosts {
		var r RunnerResult
		_ = json.Unmarshal(raw, &r)
		results[host] = r
	}
	return results
}

// RunnerSkippedEvent is emitted when a task is skipped on a host.
type RunnerSkippedEvent struct {
	EventEnvelope
	Hosts map[string]json.RawMessage `json:"hosts"`
	Task  TaskInfo                   `json:"task"`
}

// RunnerFailResult contains per-host failure data.
type RunnerFailResult struct {
	Changed bool   `json:"changed"`
	Action  string `json:"action"`
	Msg     string `json:"msg"`
	Stderr  string `json:"stderr"`
	Stdout  string `json:"stdout"`
}

// RunnerFailedEvent is emitted when a task fails on a host.
type RunnerFailedEvent struct {
	EventEnvelope
	Hosts map[string]json.RawMessage `json:"hosts"`
	Task  TaskInfo                   `json:"task"`
}

// HostResults extracts the RunnerFailResult for each host.
func (e RunnerFailedEvent) HostResults() map[string]RunnerFailResult {
	results := make(map[string]RunnerFailResult, len(e.Hosts))
	for host, raw := range e.Hosts {
		var r RunnerFailResult
		_ = json.Unmarshal(raw, &r)
		results[host] = r
	}
	return results
}

// RunnerUnreachableEvent is emitted when a host is unreachable.
type RunnerUnreachableEvent struct {
	EventEnvelope
	Hosts map[string]json.RawMessage `json:"hosts"`
	Task  TaskInfo                   `json:"task"`
}

// HostResults extracts the RunnerFailResult for each host.
func (e RunnerUnreachableEvent) HostResults() map[string]RunnerFailResult {
	results := make(map[string]RunnerFailResult, len(e.Hosts))
	for host, raw := range e.Hosts {
		var r RunnerFailResult
		_ = json.Unmarshal(raw, &r)
		results[host] = r
	}
	return results
}

// HostStats contains per-host summary statistics.
type HostStats struct {
	Ok          int `json:"ok"`
	Changed     int `json:"changed"`
	Failures    int `json:"failures"`
	Ignored     int `json:"ignored"`
	Skipped     int `json:"skipped"`
	Unreachable int `json:"unreachable"`
	Rescued     int `json:"rescued"`
}

// StatsEvent is emitted at the end of a playbook run with summary statistics.
type StatsEvent struct {
	EventEnvelope
	Stats map[string]HostStats `json:"stats"`
}
