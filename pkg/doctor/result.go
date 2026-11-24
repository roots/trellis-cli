// Package doctor checks whether a Trellis project and its servers are up to
// date. It knows nothing about specific vulnerabilities or versions: every
// check compares what is installed against what an upstream source (apt,
// php.net, Ubuntu, GitHub) already reports as current.
package doctor

type Status string

const (
	StatusOK      Status = "ok"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusUnknown Status = "unknown"
	StatusInfo    Status = "info"
	StatusSkip    Status = "skip"
)

type Result struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Status  Status   `json:"status"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

// Service is a Trellis-managed service that `trellis provision` can upgrade.
type Service string

const (
	ServiceNginx   Service = "nginx"
	ServicePHP     Service = "php"
	ServiceMariaDB Service = "mariadb"
)
