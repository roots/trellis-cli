package command

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/hashicorp/cli"
	"github.com/roots/trellis-cli/pkg/ansible/output"
)

func WithAnsibleOutput(ui cli.Ui, totalTasks int) CommandOption {
	return func(cmd *exec.Cmd) {
		playbackFile := os.Getenv("TRELLIS_CLI_PLAYBACK_FILE")

		cmd.Env = append(os.Environ(), "ANSIBLE_STDOUT_CALLBACK=ansible.posix.jsonl", "ANSIBLE_HOST_KEY_CHECKING=False")

		if playbackFile != "" {
			shPath, err := exec.LookPath("sh")
			if err != nil {
				ui.Error("Error: could not find 'sh' executable in your PATH.")
				// Set a failing command
				cmd.Path = "/bin/false"
				cmd.Args = []string{"false"}
				return
			}

			delayMs, _ := strconv.Atoi(os.Getenv("TRELLIS_CLI_PLAYBACK_DELAY"))
			delaySec := float64(delayMs) / 1000.0

			// In playback mode, we replace the ansible command with a shell script
			// that reads the playback file and streams it to stdout.
			shellCmd := fmt.Sprintf(
				`while IFS= read -r line; do echo "$line"; sleep %.4f; done < %q`,
				delaySec,
				playbackFile,
			)

			cmd.Path = shPath
			cmd.Args = []string{"sh", "-c", shellCmd}
		}

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			// ui.Error is not available here, need to think about error handling
			return
		}

		cmd.Stderr = os.Stderr

		if playbackFile != "" {
			// In playback mode, the initial totalTasks is irrelevant.
			go output.Process(stdout, 0)
		} else {
			go output.Process(stdout, totalTasks)
		}
	}
}
