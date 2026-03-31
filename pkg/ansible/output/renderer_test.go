package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/fatih/color"
)

func init() {
	// Disable color output in tests for deterministic assertions
	color.NoColor = true
}

func makeTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func TestRendererPlayStart(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, nil)

	r.OnPlayStart(PlayStartEvent{
		EventEnvelope: EventEnvelope{
			EventType: EventPlayStart,
			Timestamp: makeTime("2025-11-19T04:55:04.994964Z"),
		},
		Play: struct {
			Name     string `json:"name"`
			ID       string `json:"id"`
			Duration struct {
				Start time.Time `json:"start"`
			} `json:"duration"`
			Path string `json:"path"`
		}{
			Name: "WordPress Server: Install LEMP Stack",
			ID:   "play-1",
			Duration: struct {
				Start time.Time `json:"start"`
			}{Start: makeTime("2025-11-19T04:55:04.994789Z")},
		},
	})

	output := buf.String()
	if !strings.Contains(output, "WordPress Server: Install LEMP Stack") {
		t.Errorf("expected play name in output, got: %s", output)
	}

	if !strings.Contains(output, "▸") {
		t.Errorf("expected play indicator in output, got: %s", output)
	}
}

func TestRendererStandaloneTask(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, nil)

	// Simulate "Gathering Facts" - a task with no role prefix
	r.OnTaskStart(TaskStartEvent{
		Task: TaskInfo{
			ID:   "task-1",
			Name: "Gathering Facts",
			Duration: TaskDuration{
				Start: makeTime("2025-11-19T04:55:05.000000Z"),
			},
		},
	})

	r.OnRunnerOk(RunnerOkEvent{
		Hosts: map[string]json.RawMessage{
			"default": json.RawMessage(`{"changed":false,"action":"gather_facts"}`),
		},
		Task: TaskInfo{
			ID:   "task-1",
			Name: "Gathering Facts",
			Duration: TaskDuration{
				Start: makeTime("2025-11-19T04:55:05.000000Z"),
				End:   makeTime("2025-11-19T04:55:06.100000Z"),
			},
		},
	})

	output := buf.String()
	if !strings.Contains(output, "✓") {
		t.Errorf("expected checkmark for standalone task, got: %s", output)
	}
	if !strings.Contains(output, "Gathering Facts") {
		t.Errorf("expected task name in output, got: %s", output)
	}
}

func TestRendererRoleGrouping(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, nil)

	start := makeTime("2025-11-19T04:55:06.000000Z")

	// Task 1 in "common" role
	r.OnTaskStart(TaskStartEvent{
		Task: TaskInfo{ID: "t1", Name: "common : Validate sites", Duration: TaskDuration{Start: start}},
	})
	r.OnRunnerOk(RunnerOkEvent{
		Hosts: map[string]json.RawMessage{"default": json.RawMessage(`{"changed":false}`)},
		Task:  TaskInfo{ID: "t1", Name: "common : Validate sites", Duration: TaskDuration{Start: start, End: start.Add(100 * time.Millisecond)}},
	})

	// Task 2 in "common" role - changed
	r.OnTaskStart(TaskStartEvent{
		Task: TaskInfo{ID: "t2", Name: "common : Update apt", Duration: TaskDuration{Start: start.Add(200 * time.Millisecond)}},
	})
	r.OnRunnerOk(RunnerOkEvent{
		Hosts: map[string]json.RawMessage{"default": json.RawMessage(`{"changed":true,"action":"apt"}`)},
		Task:  TaskInfo{ID: "t2", Name: "common : Update apt", Duration: TaskDuration{Start: start.Add(200 * time.Millisecond), End: start.Add(2 * time.Second)}},
	})

	// Task 3 in "common" role - skipped
	r.OnTaskStart(TaskStartEvent{
		Task: TaskInfo{ID: "t3", Name: "common : Skip this", Duration: TaskDuration{Start: start.Add(2 * time.Second)}},
	})
	r.OnRunnerSkipped(RunnerSkippedEvent{
		Hosts: map[string]json.RawMessage{"default": json.RawMessage(`{"skipped":true}`)},
		Task:  TaskInfo{ID: "t3", Name: "common : Skip this", Duration: TaskDuration{Start: start.Add(2 * time.Second), End: start.Add(2100 * time.Millisecond)}},
	})

	// Switch to "fail2ban" role triggers finalization of "common"
	r.OnTaskStart(TaskStartEvent{
		Task: TaskInfo{ID: "t4", Name: "fail2ban : Install", Duration: TaskDuration{Start: start.Add(3 * time.Second)}},
	})
	r.OnRunnerOk(RunnerOkEvent{
		Hosts: map[string]json.RawMessage{"default": json.RawMessage(`{"changed":false}`)},
		Task:  TaskInfo{ID: "t4", Name: "fail2ban : Install", Duration: TaskDuration{Start: start.Add(3 * time.Second), End: start.Add(4 * time.Second)}},
	})

	// Finalize fail2ban via stats
	r.OnStats(StatsEvent{
		EventEnvelope: EventEnvelope{Timestamp: start.Add(5 * time.Second)},
		Stats: map[string]HostStats{
			"default": {Ok: 2, Changed: 1, Skipped: 1},
		},
	})

	output := buf.String()

	// Common role should show grouped summary
	if !strings.Contains(output, "common") {
		t.Errorf("expected 'common' role in output, got: %s", output)
	}
	if !strings.Contains(output, "1 ok") {
		t.Errorf("expected '1 ok' in common summary, got: %s", output)
	}
	if !strings.Contains(output, "1 changed") {
		t.Errorf("expected '1 changed' in common summary, got: %s", output)
	}
	if !strings.Contains(output, "1 skipped") {
		t.Errorf("expected '1 skipped' in common summary, got: %s", output)
	}

	// Changed task should be called out
	if !strings.Contains(output, "changed: Update apt") {
		t.Errorf("expected changed task callout, got: %s", output)
	}

	// fail2ban role should also appear
	if !strings.Contains(output, "fail2ban") {
		t.Errorf("expected 'fail2ban' role in output, got: %s", output)
	}

	// Summary line
	if !strings.Contains(output, "Done:") {
		t.Errorf("expected summary line, got: %s", output)
	}
}

func TestRendererFailedTask(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, nil)

	start := makeTime("2025-11-19T04:55:06.000000Z")

	r.OnTaskStart(TaskStartEvent{
		Task: TaskInfo{ID: "t1", Name: "nginx : Create config", Duration: TaskDuration{Start: start}},
	})
	r.OnRunnerFailed(RunnerFailedEvent{
		Hosts: map[string]json.RawMessage{
			"default": json.RawMessage(`{"changed":false,"action":"copy","msg":"Could not write to /etc/nginx/nginx.conf","stderr":"Permission denied"}`),
		},
		Task: TaskInfo{ID: "t1", Name: "nginx : Create config", Duration: TaskDuration{Start: start, End: start.Add(time.Second)}},
	})

	r.OnStats(StatsEvent{
		EventEnvelope: EventEnvelope{Timestamp: start.Add(2 * time.Second)},
		Stats: map[string]HostStats{
			"default": {Ok: 0, Failures: 1},
		},
	})

	output := buf.String()

	// Role should show failure icon
	if !strings.Contains(output, "✗") {
		t.Errorf("expected failure icon, got: %s", output)
	}

	// Inline failure
	if !strings.Contains(output, "FAILED: Create config") {
		t.Errorf("expected inline failure message, got: %s", output)
	}

	// End-of-run failure details
	if !strings.Contains(output, "Failures") {
		t.Errorf("expected failures section, got: %s", output)
	}
	if !strings.Contains(output, "Could not write to /etc/nginx/nginx.conf") {
		t.Errorf("expected failure msg in details, got: %s", output)
	}
	if !strings.Contains(output, "Permission denied") {
		t.Errorf("expected stderr in details, got: %s", output)
	}
}

func TestRendererMultiHost(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, nil)

	start := makeTime("2025-11-19T04:55:06.000000Z")

	r.OnTaskStart(TaskStartEvent{
		Task: TaskInfo{ID: "t1", Name: "common : Update apt", Duration: TaskDuration{Start: start}},
	})

	// Host1 changed, Host2 not changed - two separate runner events
	r.OnRunnerOk(RunnerOkEvent{
		Hosts: map[string]json.RawMessage{
			"host1.example.com": json.RawMessage(`{"changed":true,"action":"apt"}`),
		},
		Task: TaskInfo{ID: "t1", Name: "common : Update apt", Duration: TaskDuration{Start: start, End: start.Add(time.Second)}},
	})

	r.OnRunnerOk(RunnerOkEvent{
		Hosts: map[string]json.RawMessage{
			"host2.example.com": json.RawMessage(`{"changed":false,"action":"apt"}`),
		},
		Task: TaskInfo{ID: "t1", Name: "common : Update apt", Duration: TaskDuration{Start: start, End: start.Add(2 * time.Second)}},
	})

	r.OnStats(StatsEvent{
		EventEnvelope: EventEnvelope{Timestamp: start.Add(3 * time.Second)},
		Stats: map[string]HostStats{
			"host1.example.com": {Ok: 0, Changed: 1},
			"host2.example.com": {Ok: 1, Changed: 0},
		},
	})

	output := buf.String()

	// Should show host name with changed task
	if !strings.Contains(output, "host1.example.com") {
		t.Errorf("expected host name annotation for changed task, got: %s", output)
	}
}

func TestRendererParseErrorPassthrough(t *testing.T) {
	var buf bytes.Buffer
	r := NewRenderer(&buf, nil)

	r.OnParseError("PLAY [server] ***", nil)

	if !r.passthrough {
		t.Error("expected passthrough mode after parse error")
	}

	output := buf.String()
	if !strings.Contains(output, "PLAY [server] ***") {
		t.Errorf("expected raw line in output, got: %s", output)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{500 * time.Millisecond, "500ms"},
		{1100 * time.Millisecond, "1.1s"},
		{30 * time.Second, "30.0s"},
		{90 * time.Second, "1m 30s"},
		{0, "0ms"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
