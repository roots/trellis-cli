package output

import (
	"encoding/json"
	"testing"
)

func TestEventEnvelopeParsing(t *testing.T) {
	line := `{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.994964Z"}`

	var env EventEnvelope
	if err := json.Unmarshal([]byte(line), &env); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if env.EventType != EventPlayStart {
		t.Errorf("expected event type %q, got %q", EventPlayStart, env.EventType)
	}

	if env.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestPlayStartEvent(t *testing.T) {
	line := `{"_event":"v2_playbook_on_play_start","_timestamp":"2025-11-19T04:55:04.994964Z","play":{"duration":{"start":"2025-11-19T04:55:04.994789Z"},"id":"06843a54-536c-ac62-59fe-000000000003","name":"WordPress Server: Install LEMP Stack with PHP and MariaDB MySQL","path":"/dev/trellis/dev.yml:2"}}`

	var event PlayStartEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if event.Play.Name != "WordPress Server: Install LEMP Stack with PHP and MariaDB MySQL" {
		t.Errorf("unexpected play name: %s", event.Play.Name)
	}

	if event.Play.ID != "06843a54-536c-ac62-59fe-000000000003" {
		t.Errorf("unexpected play id: %s", event.Play.ID)
	}
}

func TestTaskStartEvent(t *testing.T) {
	line := `{"_event":"v2_playbook_on_task_start","_timestamp":"2025-11-19T04:55:06.134717Z","hosts":{},"task":{"duration":{"start":"2025-11-19T04:55:06.134706Z"},"id":"task-001","name":"common : Validate wordpress_sites","path":"/dev/trellis/roles/common/tasks/main.yml:2"}}`

	var event TaskStartEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if event.Task.Name != "common : Validate wordpress_sites" {
		t.Errorf("unexpected task name: %s", event.Task.Name)
	}

	if event.Task.RoleName() != "common" {
		t.Errorf("expected role name 'common', got %q", event.Task.RoleName())
	}

	if event.Task.TaskName() != "Validate wordpress_sites" {
		t.Errorf("expected task name 'Validate wordpress_sites', got %q", event.Task.TaskName())
	}
}

func TestTaskInfoRoleNameEmpty(t *testing.T) {
	task := TaskInfo{Name: "Gathering Facts"}

	if task.RoleName() != "" {
		t.Errorf("expected empty role name, got %q", task.RoleName())
	}

	if task.TaskName() != "Gathering Facts" {
		t.Errorf("expected 'Gathering Facts', got %q", task.TaskName())
	}
}

func TestRunnerOkEvent(t *testing.T) {
	line := `{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:08.153650Z","hosts":{"default":{"_ansible_no_log":false,"action":"apt","changed":true}},"task":{"duration":{"end":"2025-11-19T04:55:08.153593Z","start":"2025-11-19T04:55:06.265412Z"},"id":"task-002","name":"common : Update apt packages","path":"/dev/trellis/roles/common/tasks/main.yml:63"}}`

	var event RunnerOkEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	results := event.HostResults()
	if len(results) != 1 {
		t.Fatalf("expected 1 host result, got %d", len(results))
	}

	r, ok := results["default"]
	if !ok {
		t.Fatal("expected 'default' host")
	}

	if !r.Changed {
		t.Error("expected changed=true")
	}

	if r.Action != "apt" {
		t.Errorf("expected action 'apt', got %q", r.Action)
	}

	// Check task duration
	if event.Task.Duration.End.IsZero() {
		t.Error("expected non-zero task end time")
	}
}

func TestRunnerOkEventMultiHost(t *testing.T) {
	line := `{"_event":"v2_runner_on_ok","_timestamp":"2025-11-19T04:55:08.153650Z","hosts":{"host1.example.com":{"changed":true,"action":"apt"},"host2.example.com":{"changed":false,"action":"apt"}},"task":{"duration":{"end":"2025-11-19T04:55:08.153593Z","start":"2025-11-19T04:55:06.265412Z"},"id":"task-002","name":"common : Update apt packages"}}`

	var event RunnerOkEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	results := event.HostResults()
	if len(results) != 2 {
		t.Fatalf("expected 2 host results, got %d", len(results))
	}

	if !results["host1.example.com"].Changed {
		t.Error("expected host1 changed=true")
	}

	if results["host2.example.com"].Changed {
		t.Error("expected host2 changed=false")
	}
}

func TestRunnerSkippedEvent(t *testing.T) {
	line := `{"_event":"v2_runner_on_skipped","_timestamp":"2025-11-19T04:55:06.148712Z","hosts":{"default":{"changed":false,"skip_reason":"Conditional result was False","skipped":true}},"task":{"duration":{"end":"2025-11-19T04:55:06.148679Z","start":"2025-11-19T04:55:06.134706Z"},"id":"task-003","name":"common : Validate wordpress_sites"}}`

	var event RunnerSkippedEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(event.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(event.Hosts))
	}

	if _, ok := event.Hosts["default"]; !ok {
		t.Error("expected 'default' host")
	}
}

func TestRunnerFailedEvent(t *testing.T) {
	line := `{"_event":"v2_runner_on_failed","_timestamp":"2025-11-19T04:55:08.000000Z","hosts":{"default":{"changed":false,"action":"copy","msg":"Could not write to /etc/nginx/nginx.conf","stderr":"Permission denied"}},"task":{"duration":{"end":"2025-11-19T04:55:08.000000Z","start":"2025-11-19T04:55:07.000000Z"},"id":"task-004","name":"nginx : Create config"}}`

	var event RunnerFailedEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	results := event.HostResults()
	r := results["default"]

	if r.Msg != "Could not write to /etc/nginx/nginx.conf" {
		t.Errorf("unexpected msg: %s", r.Msg)
	}

	if r.Stderr != "Permission denied" {
		t.Errorf("unexpected stderr: %s", r.Stderr)
	}
}

func TestStatsEvent(t *testing.T) {
	line := `{"_event":"v2_playbook_on_stats","_timestamp":"2025-11-19T04:56:21.210992Z","custom_stats":{},"global_custom_stats":{},"stats":{"default":{"changed":5,"failures":0,"ignored":0,"ok":116,"rescued":0,"skipped":45,"unreachable":0}}}`

	var event StatsEvent
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	stats, ok := event.Stats["default"]
	if !ok {
		t.Fatal("expected 'default' host stats")
	}

	if stats.Ok != 116 {
		t.Errorf("expected ok=116, got %d", stats.Ok)
	}

	if stats.Changed != 5 {
		t.Errorf("expected changed=5, got %d", stats.Changed)
	}

	if stats.Skipped != 45 {
		t.Errorf("expected skipped=45, got %d", stats.Skipped)
	}

	if stats.Failures != 0 {
		t.Errorf("expected failures=0, got %d", stats.Failures)
	}
}
