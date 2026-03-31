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
	color.NoColor = true
}

// TestIntegrationFullStream simulates a complete playbook run through Parser+Renderer.
func TestIntegrationFullStream(t *testing.T) {
	events := []string{
		// Play start
		`{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.994964Z","play":{"name":"WordPress Server: Install LEMP Stack","id":"play-1","duration":{"start":"2025-11-19T04:55:04.994789Z"}}}`,

		// Gathering Facts (standalone task)
		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:05.024263Z","task":{"id":"t0","name":"Gathering Facts","duration":{"start":"2025-11-19T04:55:05.024249Z"}}}`,
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:06.126433Z","hosts":{"default":{"changed":false,"action":"gather_facts"}},"task":{"id":"t0","name":"Gathering Facts","duration":{"start":"2025-11-19T04:55:05.024249Z","end":"2025-11-19T04:55:06.126433Z"}}}`,

		// common role - 3 tasks: 1 ok, 1 changed, 1 skipped
		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:06.134717Z","task":{"id":"t1","name":"common : Validate sites","duration":{"start":"2025-11-19T04:55:06.134706Z"}}}`,
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:06.150000Z","hosts":{"default":{"changed":false,"action":"assert"}},"task":{"id":"t1","name":"common : Validate sites","duration":{"start":"2025-11-19T04:55:06.134706Z","end":"2025-11-19T04:55:06.150000Z"}}}`,

		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:06.200000Z","task":{"id":"t2","name":"common : Update apt","duration":{"start":"2025-11-19T04:55:06.200000Z"}}}`,
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:08.000000Z","hosts":{"default":{"changed":true,"action":"apt"}},"task":{"id":"t2","name":"common : Update apt","duration":{"start":"2025-11-19T04:55:06.200000Z","end":"2025-11-19T04:55:08.000000Z"}}}`,

		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:08.100000Z","task":{"id":"t3","name":"common : Maybe skip","duration":{"start":"2025-11-19T04:55:08.100000Z"}}}`,
		`{"_event":"v2_runner_on_skipped","_timestamp":"2025-11-19T04:55:08.200000Z","hosts":{"default":{"skipped":true}},"task":{"id":"t3","name":"common : Maybe skip","duration":{"start":"2025-11-19T04:55:08.100000Z","end":"2025-11-19T04:55:08.200000Z"}}}`,

		// nginx role - 2 tasks: 1 ok, 1 failed
		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:09.000000Z","task":{"id":"t4","name":"nginx : Install","duration":{"start":"2025-11-19T04:55:09.000000Z"}}}`,
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:10.000000Z","hosts":{"default":{"changed":false,"action":"apt"}},"task":{"id":"t4","name":"nginx : Install","duration":{"start":"2025-11-19T04:55:09.000000Z","end":"2025-11-19T04:55:10.000000Z"}}}`,

		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:10.100000Z","task":{"id":"t5","name":"nginx : Create config","duration":{"start":"2025-11-19T04:55:10.100000Z"}}}`,
		`{"_event":"v2_runner_on_failed","_timestamp":"2025-11-19T04:55:11.000000Z","hosts":{"default":{"changed":false,"action":"copy","msg":"Could not write to /etc/nginx/nginx.conf","stderr":"Permission denied"}},"task":{"id":"t5","name":"nginx : Create config","duration":{"start":"2025-11-19T04:55:10.100000Z","end":"2025-11-19T04:55:11.000000Z"}}}`,

		// Stats
		`{"_event":"v2_playbook_on_stats","_timestamp":"2025-11-19T04:55:12.000000Z","stats":{"default":{"ok":3,"changed":1,"failures":1,"skipped":1,"unreachable":0,"ignored":0,"rescued":0}}}`,
	}

	input := strings.Join(events, "\n")

	var buf bytes.Buffer
	renderer := NewRenderer(&buf, nil)
	parser := NewParser(renderer, &buf)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	output := buf.String()

	// Play name
	if !strings.Contains(output, "WordPress Server: Install LEMP Stack") {
		t.Errorf("missing play name in output:\n%s", output)
	}

	// Standalone task
	if !strings.Contains(output, "Gathering Facts") {
		t.Errorf("missing standalone task in output:\n%s", output)
	}

	// common role with stats
	if !strings.Contains(output, "common") {
		t.Errorf("missing common role in output:\n%s", output)
	}
	if !strings.Contains(output, "1 changed") {
		t.Errorf("missing changed count in output:\n%s", output)
	}
	if !strings.Contains(output, "changed: Update apt") {
		t.Errorf("missing changed task callout in output:\n%s", output)
	}

	// nginx role with failure
	if !strings.Contains(output, "nginx") {
		t.Errorf("missing nginx role in output:\n%s", output)
	}
	if !strings.Contains(output, "1 failed") {
		t.Errorf("missing failed count in output:\n%s", output)
	}
	if !strings.Contains(output, "FAILED: Create config") {
		t.Errorf("missing inline failure in output:\n%s", output)
	}

	// End-of-run failure details
	if !strings.Contains(output, "Failures") {
		t.Errorf("missing failures section in output:\n%s", output)
	}
	if !strings.Contains(output, "Could not write to /etc/nginx/nginx.conf") {
		t.Errorf("missing failure msg in output:\n%s", output)
	}
	if !strings.Contains(output, "Permission denied") {
		t.Errorf("missing stderr in failure details:\n%s", output)
	}

	// Summary line
	if !strings.Contains(output, "Done:") {
		t.Errorf("missing summary line in output:\n%s", output)
	}

	t.Logf("Full output:\n%s", output)
}

// TestIntegrationMultiHost tests multi-host rendering.
func TestIntegrationMultiHost(t *testing.T) {
	events := []string{
		`{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.000000Z","play":{"name":"Multi Host Play","id":"play-1","duration":{"start":"2025-11-19T04:55:04.000000Z"}}}`,

		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:05.000000Z","task":{"id":"t1","name":"common : Update apt","duration":{"start":"2025-11-19T04:55:05.000000Z"}}}`,

		// Host1 changed
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:06.000000Z","hosts":{"host1.example.com":{"changed":true,"action":"apt"}},"task":{"id":"t1","name":"common : Update apt","duration":{"start":"2025-11-19T04:55:05.000000Z","end":"2025-11-19T04:55:06.000000Z"}}}`,
		// Host2 ok
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:06.500000Z","hosts":{"host2.example.com":{"changed":false,"action":"apt"}},"task":{"id":"t1","name":"common : Update apt","duration":{"start":"2025-11-19T04:55:05.000000Z","end":"2025-11-19T04:55:06.500000Z"}}}`,

		`{"_event":"v2_playbook_on_stats","_timestamp":"2025-11-19T04:55:07.000000Z","stats":{"host1.example.com":{"ok":0,"changed":1,"failures":0,"skipped":0,"unreachable":0},"host2.example.com":{"ok":1,"changed":0,"failures":0,"skipped":0,"unreachable":0}}}`,
	}

	input := strings.Join(events, "\n")

	var buf bytes.Buffer
	renderer := NewRenderer(&buf, nil)
	parser := NewParser(renderer, &buf)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	output := buf.String()

	// Multi-host changed task should show hostname
	if !strings.Contains(output, "host1.example.com") {
		t.Errorf("expected host name annotation for changed task:\n%s", output)
	}

	t.Logf("Full output:\n%s", output)
}

// TestIntegrationParseErrorFallback tests graceful fallback on non-JSON input.
func TestIntegrationParseErrorFallback(t *testing.T) {
	input := strings.Join([]string{
		`{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.000000Z","play":{"name":"Test","id":"1","duration":{"start":"2025-11-19T04:55:04.000000Z"}}}`,
		`PLAY [server] ***************************************************`,
		`TASK [common : Do something] *****`,
		`ok: [default]`,
	}, "\n")

	var buf bytes.Buffer
	renderer := NewRenderer(&buf, nil)
	parser := NewParser(renderer, &buf)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	output := buf.String()

	// Should have rendered play start before fallback
	if !strings.Contains(output, "Test") {
		t.Errorf("expected play name before fallback:\n%s", output)
	}

	// Raw lines should pass through
	if !strings.Contains(output, "TASK [common : Do something]") {
		t.Errorf("expected raw Ansible output in passthrough:\n%s", output)
	}
	if !strings.Contains(output, "ok: [default]") {
		t.Errorf("expected raw task result in passthrough:\n%s", output)
	}
}

// TestIntegrationUnreachableHost tests that an unreachable host on a roleless task
// (like Gathering Facts) renders correctly without producing broken output.
func TestIntegrationUnreachableHost(t *testing.T) {
	events := []string{
		`{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.000000Z","play":{"name":"WordPress Server: Install LEMP Stack","id":"play-1","duration":{"start":"2025-11-19T04:55:04.000000Z"}}}`,
		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:05.000000Z","task":{"id":"t0","name":"Gathering Facts","duration":{"start":"2025-11-19T04:55:05.000000Z"}}}`,
		`{"_event":"v2_runner_on_unreachable","_timestamp":"2025-11-19T04:55:05.200000Z","hosts":{"default":{"changed":false,"msg":"Failed to connect to the host via ssh: ssh: connect to host 127.0.0.1 port 57819: Connection refused","unreachable":true}},"task":{"id":"t0","name":"Gathering Facts","duration":{"start":"2025-11-19T04:55:05.000000Z","end":"2025-11-19T04:55:05.200000Z"}}}`,
		`{"_event":"v2_playbook_on_stats","_timestamp":"2025-11-19T04:55:05.300000Z","stats":{"default":{"ok":0,"changed":0,"failures":0,"skipped":0,"unreachable":1}}}`,
	}

	input := strings.Join(events, "\n")

	var buf bytes.Buffer
	renderer := NewRenderer(&buf, nil)
	parser := NewParser(renderer, &buf)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	output := buf.String()

	// Should show failed standalone task with cross icon
	if !strings.Contains(output, "✗") {
		t.Errorf("expected failure icon for unreachable task:\n%s", output)
	}
	if !strings.Contains(output, "Gathering Facts") {
		t.Errorf("expected 'Gathering Facts' in output:\n%s", output)
	}

	// Should show inline failure
	if !strings.Contains(output, "FAILED: Gathering Facts") {
		t.Errorf("expected inline failure for unreachable task:\n%s", output)
	}
	if !strings.Contains(output, "Connection refused") {
		t.Errorf("expected connection error message:\n%s", output)
	}

	// Should show end-of-run failure details
	if !strings.Contains(output, "Failures") {
		t.Errorf("expected failures section:\n%s", output)
	}

	// Summary should show unreachable count
	if !strings.Contains(output, "1 unreachable") {
		t.Errorf("expected '1 unreachable' in summary:\n%s", output)
	}

	// Should NOT have an empty role name (the original bug)
	if strings.Contains(output, "  ✗                ") {
		t.Errorf("found empty role name in output (original bug):\n%s", output)
	}

	t.Logf("Full output:\n%s", output)
}

// TestIntegrationWithTaskList tests that pre-populated task list provides correct progress.
func TestIntegrationWithTaskList(t *testing.T) {
	taskList := &TaskList{
		Total:   6,
		PerRole: map[string]int{"": 1, "common": 3, "nginx": 2},
	}

	events := []string{
		`{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.994964Z","play":{"name":"Test","id":"play-1","duration":{"start":"2025-11-19T04:55:04.994789Z"}}}`,
		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:05.000000Z","task":{"id":"t0","name":"Gathering Facts","duration":{"start":"2025-11-19T04:55:05.000000Z"}}}`,
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:06.000000Z","hosts":{"default":{"changed":false}},"task":{"id":"t0","name":"Gathering Facts","duration":{"start":"2025-11-19T04:55:05.000000Z","end":"2025-11-19T04:55:06.000000Z"}}}`,
		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:06.100000Z","task":{"id":"t1","name":"common : Task1","duration":{"start":"2025-11-19T04:55:06.100000Z"}}}`,
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:07.000000Z","hosts":{"default":{"changed":false}},"task":{"id":"t1","name":"common : Task1","duration":{"start":"2025-11-19T04:55:06.100000Z","end":"2025-11-19T04:55:07.000000Z"}}}`,
		`{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:07.100000Z","task":{"id":"t2","name":"common : Task2","duration":{"start":"2025-11-19T04:55:07.100000Z"}}}`,
		`{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:08.000000Z","hosts":{"default":{"changed":false}},"task":{"id":"t2","name":"common : Task2","duration":{"start":"2025-11-19T04:55:07.100000Z","end":"2025-11-19T04:55:08.000000Z"}}}`,
		`{"_event":"v2_playbook_on_stats","_timestamp":"2025-11-19T04:55:09.000000Z","stats":{"default":{"ok":3,"changed":0,"failures":0,"skipped":0,"unreachable":0}}}`,
	}

	input := strings.Join(events, "\n")

	var buf bytes.Buffer
	renderer := NewRenderer(&buf, taskList)
	parser := NewParser(renderer, &buf)

	if err := parser.Parse(strings.NewReader(input)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	output := buf.String()

	// With task list, global progress bar should show out of 6 total (from --list-tasks)
	if !strings.Contains(output, "/6") {
		t.Errorf("expected global total of 6 from task list, got:\n%s", output)
	}

	// Progress bar should show filled portion (━) and unfilled portion (─)
	if !strings.Contains(output, "━") || !strings.Contains(output, "─") {
		t.Errorf("expected progress bar characters in output, got:\n%s", output)
	}

	t.Logf("Full output:\n%s", output)
}

// Avoid unused import
var _ = json.Marshal
var _ = time.Now
