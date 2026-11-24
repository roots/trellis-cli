package doctor

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

//go:embed playbook.yml
var PlaybookYml string

const (
	// FilePrefix starts the names of the temporary playbook and probe files
	// written to the project directory while doctor runs.
	FilePrefix = "trellis_cli_doctor_"
	// ProbeVar is the extra var giving the playbook the probe's file name.
	ProbeVar = "trellis_cli_doctor_probe"

	varsTaskName  = "Trellis CLI: Doctor vars"
	probeTaskName = "Trellis CLI: Doctor probe"
)

// ErrNoHosts means no host is in both the web group and the environment's
// group in the inventory.
var ErrNoHosts = errors.New("no hosts in the inventory's web group for this environment")

// hostRun is the playbook's outcome for one host.
type hostRun struct {
	Name    string
	Address string
	User    string
	Vars    projectVars
	Probe   string
	// Error is set when the host couldn't be probed.
	Error string
}

type ansibleOutput struct {
	Plays []struct {
		Tasks []struct {
			Task struct {
				Name string `json:"name"`
			} `json:"task"`
			Hosts map[string]taskResult `json:"hosts"`
		} `json:"tasks"`
	} `json:"plays"`
	Stats map[string]json.RawMessage `json:"stats"`
}

type taskResult struct {
	Stdout      string          `json:"stdout"`
	Stderr      string          `json:"stderr"`
	Msg         json.RawMessage `json:"msg"`
	Rc          *int            `json:"rc"`
	Failed      bool            `json:"failed"`
	Unreachable bool            `json:"unreachable"`
}

// varsMsg is the vars task's message. The playbook converts each value to a
// string.
type varsMsg struct {
	Vars projectVars `json:"vars"`
	Host string      `json:"host"`
	User string      `json:"user"`
}

// parsePlaybookOutput parses ansible-playbook output from the json stdout
// callback into one hostRun per host, sorted by name.
func parsePlaybookOutput(stdout []byte) ([]hostRun, error) {
	var out ansibleOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return nil, fmt.Errorf("couldn't parse ansible-playbook output: %w", err)
	}

	if len(out.Stats) == 0 {
		return nil, ErrNoHosts
	}

	runs := map[string]*hostRun{}
	varsRead := map[string]bool{}
	for name := range out.Stats {
		runs[name] = &hostRun{Name: name}
	}

	for _, play := range out.Plays {
		for _, task := range play.Tasks {
			for host, res := range task.Hosts {
				run, ok := runs[host]
				if !ok {
					continue
				}

				if (res.Failed || res.Unreachable) && run.Error == "" {
					run.Error = resultMessage(res)
				}

				switch task.Task.Name {
				case varsTaskName:
					var msg varsMsg
					if err := json.Unmarshal(res.Msg, &msg); err == nil {
						run.Vars, run.Address, run.User = msg.Vars, msg.Host, msg.User
						varsRead[host] = true
					}
				case probeTaskName:
					if !res.Failed && !res.Unreachable {
						run.Probe = res.Stdout
					}
				}
			}
		}
	}

	names := slices.Sorted(maps.Keys(runs))

	result := make([]hostRun, 0, len(names))
	for _, name := range names {
		run := runs[name]
		switch {
		case run.Error != "":
		case run.Probe == "":
			run.Error = "probe didn't run"
		case !varsRead[name]:
			// Without the project's package names and versions, packages
			// would look uninstalled.
			run.Error = "couldn't read the project's variables for this host"
		}
		result = append(result, *run)
	}

	return result, nil
}

func resultMessage(res taskResult) string {
	var msg string
	if err := json.Unmarshal(res.Msg, &msg); err != nil || msg == "" {
		msg = strings.TrimSpace(string(res.Msg))
	}

	if msg == "" || msg == "null" {
		msg = strings.TrimSpace(res.Stderr)
	}

	if msg == "" && res.Rc != nil {
		msg = fmt.Sprintf("exited with status %d", *res.Rc)
	}

	if msg == "" {
		msg = "failed"
	}

	// Some failures (e.g. Trellis's host key check) include pages of
	// verbose ssh output; the first lines say what happened.
	const maxLines = 8
	lines := strings.Split(strings.TrimSpace(msg), "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], "…")
	}

	return strings.Join(lines, "\n")
}
