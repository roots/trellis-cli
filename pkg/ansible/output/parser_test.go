package output

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// mockHandler records all events dispatched by the parser.
type mockHandler struct {
	playStarts  []PlayStartEvent
	taskStarts  []TaskStartEvent
	runnerOks   []RunnerOkEvent
	skipped     []RunnerSkippedEvent
	failed      []RunnerFailedEvent
	unreachable []RunnerUnreachableEvent
	stats       []StatsEvent
	parseErrors []string
}

func (m *mockHandler) OnPlayStart(e PlayStartEvent)           { m.playStarts = append(m.playStarts, e) }
func (m *mockHandler) OnTaskStart(e TaskStartEvent)           { m.taskStarts = append(m.taskStarts, e) }
func (m *mockHandler) OnRunnerOk(e RunnerOkEvent)             { m.runnerOks = append(m.runnerOks, e) }
func (m *mockHandler) OnRunnerSkipped(e RunnerSkippedEvent)   { m.skipped = append(m.skipped, e) }
func (m *mockHandler) OnRunnerFailed(e RunnerFailedEvent)     { m.failed = append(m.failed, e) }
func (m *mockHandler) OnRunnerUnreachable(e RunnerUnreachableEvent) {
	m.unreachable = append(m.unreachable, e)
}
func (m *mockHandler) OnStats(e StatsEvent)              { m.stats = append(m.stats, e) }
func (m *mockHandler) OnParseError(line string, err error) { m.parseErrors = append(m.parseErrors, line) }

func TestParserBasicEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.994964Z","play":{"name":"Test Play","id":"play-1","duration":{"start":"2025-11-19T04:55:04.994789Z"}}}`,
		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:05.024263Z","task":{"id":"task-1","name":"common : Do something","duration":{"start":"2025-11-19T04:55:05.024249Z"}}}`,
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:06.126433Z","hosts":{"default":{"changed":false,"action":"setup"}},"task":{"id":"task-1","name":"common : Do something","duration":{"start":"2025-11-19T04:55:05.024249Z","end":"2025-11-19T04:55:06.126433Z"}}}`,
		`{"_event":"v2_runner_on_skipped","_timestamp":"2025-11-19T04:55:06.148712Z","hosts":{"default":{"skipped":true}},"task":{"id":"task-2","name":"common : Maybe skip"}}`,
		`{"_event":"v2_playbook_on_stats","_timestamp":"2025-11-19T04:56:21.210992Z","stats":{"default":{"ok":1,"changed":0,"failures":0,"skipped":1,"unreachable":0}}}`,
	}, "\n")

	handler := &mockHandler{}
	parser := NewParser(handler, os.Stdout)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(handler.playStarts) != 1 {
		t.Errorf("expected 1 play start, got %d", len(handler.playStarts))
	}

	if handler.playStarts[0].Play.Name != "Test Play" {
		t.Errorf("unexpected play name: %s", handler.playStarts[0].Play.Name)
	}

	if len(handler.taskStarts) != 1 {
		t.Errorf("expected 1 task start, got %d", len(handler.taskStarts))
	}

	if len(handler.runnerOks) != 1 {
		t.Errorf("expected 1 runner ok, got %d", len(handler.runnerOks))
	}

	if len(handler.skipped) != 1 {
		t.Errorf("expected 1 skipped, got %d", len(handler.skipped))
	}

	if len(handler.stats) != 1 {
		t.Errorf("expected 1 stats, got %d", len(handler.stats))
	}

	if len(handler.parseErrors) != 0 {
		t.Errorf("expected 0 parse errors, got %d", len(handler.parseErrors))
	}
}

func TestParserFailedAndUnreachable(t *testing.T) {
	input := strings.Join([]string{
		`{"_event":"v2_runner_on_failed","_timestamp":"2025-11-19T04:55:08.000000Z","hosts":{"default":{"changed":false,"msg":"Something broke","stderr":"error details"}},"task":{"id":"task-1","name":"nginx : Create config"}}`,
		`{"_event":"v2_runner_on_unreachable","_timestamp":"2025-11-19T04:55:09.000000Z","hosts":{"web1":{"msg":"Connection refused"}},"task":{"id":"task-2","name":"common : Check host"}}`,
	}, "\n")

	handler := &mockHandler{}
	parser := NewParser(handler, os.Stdout)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(handler.failed) != 1 {
		t.Errorf("expected 1 failed, got %d", len(handler.failed))
	}

	if len(handler.unreachable) != 1 {
		t.Errorf("expected 1 unreachable, got %d", len(handler.unreachable))
	}
}

func TestParserParseErrorFallsBackToPassthrough(t *testing.T) {
	input := strings.Join([]string{
		`{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.994964Z","play":{"name":"Test","id":"1"}}`,
		`This is not JSON - standard Ansible output`,
		`TASK [common : Do something] **********`,
		`ok: [default]`,
	}, "\n")

	var passthrough bytes.Buffer
	handler := &mockHandler{}
	parser := NewParser(handler, &passthrough)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// First event should parse fine
	if len(handler.playStarts) != 1 {
		t.Errorf("expected 1 play start, got %d", len(handler.playStarts))
	}

	// Second line triggers parse error
	if len(handler.parseErrors) != 1 {
		t.Errorf("expected 1 parse error, got %d", len(handler.parseErrors))
	}

	// Remaining lines go to passthrough
	passthroughOutput := passthrough.String()
	if !strings.Contains(passthroughOutput, "TASK [common : Do something]") {
		t.Error("expected passthrough to contain raw Ansible output")
	}
	if !strings.Contains(passthroughOutput, "ok: [default]") {
		t.Error("expected passthrough to contain 'ok: [default]'")
	}
}

func TestParserUnknownEventsIgnored(t *testing.T) {
	input := `{"_event":"v2_some_unknown_event","_timestamp":"2025-11-19T04:55:04.994964Z","data":"stuff"}`

	handler := &mockHandler{}
	parser := NewParser(handler, os.Stdout)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// No events dispatched, no errors
	if len(handler.parseErrors) != 0 {
		t.Errorf("expected 0 parse errors for unknown events, got %d", len(handler.parseErrors))
	}
}

func TestParserEmptyInput(t *testing.T) {
	handler := &mockHandler{}
	parser := NewParser(handler, os.Stdout)

	if err := parser.Parse(strings.NewReader("")); err != nil {
		t.Fatalf("parse error: %v", err)
	}
}
