package ansible

import (
	"regexp"
	"strings"

	"github.com/roots/trellis-cli/command"
)

func GetTaskCount(playbook Playbook) (int, error) {
	playbook.AddFlag("--list-tasks")
	cmd := command.Cmd("ansible-playbook", playbook.CmdArgs())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, err
	}

	return countTasks(string(output)), nil
}

func countTasks(output string) int {
	lines := strings.Split(output, "\n")
	inTasksSection := false
	count := 0
	// This regex is brittle and might need to be adjusted
	taskRegex := regexp.MustCompile(`^\s+([\w\.-]+)\s*:\s*(.*)$`)

	for _, line := range lines {
		if strings.HasSuffix(strings.TrimSpace(line), "tasks:") {
			inTasksSection = true
			continue
		}

		if inTasksSection {
			if taskRegex.MatchString(line) {
				count++
			} else if !strings.HasPrefix(line, " ") && len(strings.TrimSpace(line)) > 0 {
				inTasksSection = false
			}
		}
	}
	return count
}
