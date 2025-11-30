package output

import (
	"encoding/json"
	"time"
)

type Event struct {
	Event     string    `json:"_event"`
	Timestamp time.Time `json:"_timestamp"`
}

type Task struct {
	Duration struct {
		Start time.Time `json:"start"`
		End   time.Time `json:"end"`
	} `json:"duration"`
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type Play struct {
	Duration struct {
		Start time.Time `json:"start"`
	} `json:"duration"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type PlaybookOnPlayStartEvent struct {
	Event
	Play Play `json:"play"`
}

type PlaybookOnTaskStartEvent struct {
	Event
	Hosts map[string]interface{} `json:"hosts"`
	Task  Task                   `json:"task"`
}

type HostResult struct {
	Changed bool            `json:"changed"`
	Msg     string          `json:"msg"`
	Stdout  string          `json:"stdout"`
	Stderr  string          `json:"stderr"`
	Results json.RawMessage `json:"results,omitempty"`
}

type RunnerOnOkEvent struct {
	Event
	Hosts map[string]HostResult `json:"hosts"`
	Task  Task                  `json:"task"`
}

type RunnerOnFailedEvent struct {
	Event
	Hosts map[string]HostResult `json:"hosts"`
	Task  Task                  `json:"task"`
}

// RunnerOnSkippedEvent represents a v2_runner_on_skipped event.
type RunnerOnSkippedEvent struct {
	Event
	Hosts map[string]struct {
		AnsibleNoLog   bool   `json:"_ansible_no_log"`
		Action         string `json:"action"`
		Changed        bool   `json:"changed"`
		FalseCondition string `json:"false_condition"`
		SkipReason     string `json:"skip_reason"`
		Skipped        bool   `json:"skipped"`
		Msg            string `json:"msg"`
		Results        []struct {
			AnsibleItemLabel json.RawMessage `json:"_ansible_item_label"`
			AnsibleNoLog     bool            `json:"_ansible_no_log"`
			AnsibleLoopVar   string          `json:"ansible_loop_var"`
			Changed          bool            `json:"changed"`
			FalseCondition   string          `json:"false_condition"`
			Item             json.RawMessage `json:"item"`
			SkipReason       string          `json:"skip_reason"`
			Skipped          bool            `json:"skipped"`
		} `json:"results"`
	} `json:"hosts"`
	Task Task `json:"task"`
}

type PlaybookOnStatsEvent struct {
	Event
	Stats map[string]struct {
		Changed     int `json:"changed"`
		Failures    int `json:"failures"`
		Ok          int `json:"ok"`
		Skipped     int `json:"skipped"`
		Unreachable int `json:"unreachable"`
	} `json:"stats"`
}
