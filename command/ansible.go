package command

import (
	"os"
	"os/exec"

	"github.com/roots/trellis-cli/pkg/ansible/output"
	"github.com/hashicorp/cli"
)

func WithAnsibleOutput(ui cli.Ui, totalTasks int) CommandOption {
	return func(cmd *exec.Cmd) {
		cmd.Env = append(os.Environ(), "ANSIBLE_STDOUT_CALLBACK=ansible.posix.jsonl", "ANSIBLE_HOST_KEY_CHECKING=False")

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			// ui.Error is not available here, need to think about error handling
			return
		}

		cmd.Stderr = os.Stderr

		go output.Process(stdout, totalTasks)
	}
}
