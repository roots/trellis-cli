package output

import (
	"bufio"
	"io"
	"os/exec"
	"strings"
)

// TaskList holds the parsed result of ansible-playbook --list-tasks.
type TaskList struct {
	Total    int
	PerRole  map[string]int // role name -> task count ("" for roleless tasks)
}

// ListTasks runs ansible-playbook --list-tasks with the same args and parses the output.
// Returns nil if the command fails (non-fatal — we just won't have an initial total).
func ListTasks(cmdPath string, args []string) *TaskList {
	listArgs := make([]string, len(args))
	copy(listArgs, args)
	listArgs = append(listArgs, "--list-tasks")

	cmd := exec.Command(cmdPath, listArgs...)
	stdout, err := cmd.Output()
	if err != nil {
		return nil
	}

	return parseTaskList(strings.NewReader(string(stdout)))
}

// parseTaskList parses the output of ansible-playbook --list-tasks.
//
// Format:
//
//	playbook: server.yml
//
//	  play #1 (localhost): Play Name	TAGS: []
//	    tasks:
//	      Gathering Facts	TAGS: []
//	      common : Validate sites	TAGS: [common]
//	      include_tasks	TAGS: [composer]
func parseTaskList(r io.Reader) *TaskList {
	tl := &TaskList{
		PerRole: make(map[string]int),
	}

	scanner := bufio.NewScanner(r)
	inTasks := false

	for scanner.Scan() {
		line := scanner.Text()

		// "    tasks:" marks the start of a task list for a play
		trimmed := strings.TrimSpace(line)
		if trimmed == "tasks:" {
			inTasks = true
			continue
		}

		// Empty line or new play ends task list
		if trimmed == "" || strings.HasPrefix(trimmed, "play #") || strings.HasPrefix(trimmed, "playbook:") {
			inTasks = false
			continue
		}

		if !inTasks {
			continue
		}

		// Strip TAGS suffix: "task name\tTAGS: [...]"
		taskName := trimmed
		if idx := strings.Index(taskName, "\t"); idx != -1 {
			taskName = taskName[:idx]
		}

		// Skip bare include_tasks / include_role (dynamic, will be resolved at runtime)
		if taskName == "include_tasks" || taskName == "include_role" {
			continue
		}

		tl.Total++

		// Extract role name
		roleName := ""
		if parts := strings.SplitN(taskName, " : ", 2); len(parts) == 2 {
			roleName = parts[0]
		}
		tl.PerRole[roleName]++
	}

	return tl
}
