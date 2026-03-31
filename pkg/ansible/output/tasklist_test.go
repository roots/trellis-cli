package output

import (
	"strings"
	"testing"
)

func TestParseTaskList(t *testing.T) {
	input := `
playbook: server.yml

  play #1 (localhost): Ensure necessary variables are defined	TAGS: []
    tasks:
      Ensure environment is defined	TAGS: [variable-check]

  play #2 (web:&production): Test Connection	TAGS: []
    tasks:
      connection : Require manual definition of remote-user	TAGS: [always, connection]
      connection : Check whether Ansible can connect	TAGS: [always, connection, connection-tests]
      connection : Set remote user for each host	TAGS: [always, connection]

  play #3 (web:&production): WordPress Server	TAGS: []
    tasks:
      Gathering Facts	TAGS: []
      common : Validate sites	TAGS: [common]
      common : Update apt	TAGS: [common]
      fail2ban : Install	TAGS: [fail2ban]
      composer : Install	TAGS: [composer]
      include_tasks	TAGS: [composer]
      include_tasks	TAGS: [composer]
      wp-cli : Install	TAGS: [wp-cli]
`

	tl := parseTaskList(strings.NewReader(input))

	if tl == nil {
		t.Fatal("expected non-nil TaskList")
	}

	// 1 (Ensure environment) + 3 (connection) + 1 (Gathering Facts) + 2 (common) + 1 (fail2ban) + 1 (composer) + 1 (wp-cli) = 10
	// include_tasks are skipped
	expectedTotal := 10
	if tl.Total != expectedTotal {
		t.Errorf("expected total %d, got %d", expectedTotal, tl.Total)
	}

	// Roleless tasks: "Ensure environment is defined" + "Gathering Facts"
	if tl.PerRole[""] != 2 {
		t.Errorf("expected 2 roleless tasks, got %d", tl.PerRole[""])
	}

	if tl.PerRole["connection"] != 3 {
		t.Errorf("expected 3 connection tasks, got %d", tl.PerRole["connection"])
	}

	if tl.PerRole["common"] != 2 {
		t.Errorf("expected 2 common tasks, got %d", tl.PerRole["common"])
	}

	if tl.PerRole["fail2ban"] != 1 {
		t.Errorf("expected 1 fail2ban task, got %d", tl.PerRole["fail2ban"])
	}

	// include_tasks should not be counted
	if _, ok := tl.PerRole["include_tasks"]; ok {
		t.Error("include_tasks should not appear as a role")
	}
}

func TestParseTaskListWithRealData(t *testing.T) {
	// Counts from the full tasks.txt file
	// This tests that the parser handles the real-world format correctly
	input := `
playbook: server.yml

  play #1 (localhost): Ensure necessary variables are defined	TAGS: []
    tasks:
      Ensure environment is defined	TAGS: [variable-check]

  play #2 (web:&production): Test Connection and Determine Remote User	TAGS: []
    tasks:
      connection : Require manual definition	TAGS: [always, connection]
      connection : Check connect	TAGS: [always, connection, connection-tests]
      connection : Warn about host keys	TAGS: [always, connection, connection-tests]
      connection : Set remote user	TAGS: [always, connection]
      connection : Announce user	TAGS: [always, connection]
      connection : Load become password	TAGS: [always, connection]
`

	tl := parseTaskList(strings.NewReader(input))

	if tl.Total != 7 {
		t.Errorf("expected total 7, got %d", tl.Total)
	}

	if tl.PerRole["connection"] != 6 {
		t.Errorf("expected 6 connection tasks, got %d", tl.PerRole["connection"])
	}
}

func TestParseTaskListEmpty(t *testing.T) {
	tl := parseTaskList(strings.NewReader(""))

	if tl.Total != 0 {
		t.Errorf("expected total 0, got %d", tl.Total)
	}
}
