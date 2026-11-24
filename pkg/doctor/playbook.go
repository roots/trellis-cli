package doctor

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
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

// HostRun is the playbook's outcome for one host.
type HostRun struct {
	Name    string      `json:"name"`
	Address string      `json:"address,omitempty"`
	User    string      `json:"user,omitempty"`
	Vars    ProjectVars `json:"-"`
	Probe   string      `json:"-"`
	// Error is set when the host couldn't be probed.
	Error string `json:"error,omitempty"`
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

// varsMsg is the vars task's message. Values are decoded loosely: the
// playbook converts them to strings, but older or customized setups could
// still report numbers (e.g. an unquoted `php_version: 8.3`).
type varsMsg struct {
	Vars map[string]any `json:"vars"`
	Host any            `json:"host"`
	User any            `json:"user"`
}

func (m varsMsg) projectVars() ProjectVars {
	return ProjectVars{
		PHPVersion:           scalarString(m.Vars["php_version"]),
		MariaDBVersion:       scalarString(m.Vars["mariadb_version"]),
		NginxPackage:         scalarString(m.Vars["nginx_package"]),
		MariaDBServerPackage: scalarString(m.Vars["mariadb_server_package"]),
	}
}

func scalarString(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

// ParsePlaybookOutput parses ansible-playbook output from the json stdout
// callback into one HostRun per host, sorted by name.
func ParsePlaybookOutput(stdout []byte) ([]HostRun, error) {
	var out ansibleOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return nil, fmt.Errorf("couldn't parse ansible-playbook output: %w", err)
	}

	if len(out.Stats) == 0 {
		return nil, ErrNoHosts
	}

	runs := map[string]*HostRun{}
	for name := range out.Stats {
		runs[name] = &HostRun{Name: name}
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
						run.Vars = msg.projectVars()
						run.Address, run.User = scalarString(msg.Host), scalarString(msg.User)
					}
				case probeTaskName:
					if !res.Failed && !res.Unreachable {
						run.Probe = res.Stdout
					}
				}
			}
		}
	}

	names := make([]string, 0, len(runs))
	for name := range runs {
		names = append(names, name)
	}
	sort.Strings(names)

	result := make([]HostRun, 0, len(names))
	for _, name := range names {
		run := runs[name]
		if run.Probe == "" && run.Error == "" {
			run.Error = "probe didn't run"
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

	return strings.TrimSpace(msg)
}
