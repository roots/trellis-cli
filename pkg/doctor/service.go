package doctor

import "time"

// Service is a Trellis-managed service that doctor checks and
// `trellis provision` can upgrade.
type Service struct {
	// Tag is the service's Trellis role tag.
	Tag  string
	Name string
	// VersionVar is the project variable that picks the service's version
	// series, e.g. php_version.
	VersionVar string
	// UpgradeVars make the role upgrade its packages. Role-specific vars are
	// used where they exist because they take precedence over
	// apt_package_state if a project sets them.
	UpgradeVars []string
	// UpgradeNote is shown with a fix that upgrades the service.
	UpgradeNote string
	// LatestRelease returns the latest upstream release of a branch (e.g.
	// "8.3"), or nil if upstream doesn't know it. Only set where upstream
	// releases can reach servers before their apt source does (PHP).
	LatestRelease func(branch string) (*Release, error)
}

// Release is the latest upstream release of a branch.
type Release struct {
	Version  string
	Date     time.Time
	Security bool
	// Supported is nil when upstream didn't say.
	Supported *bool
}

var (
	nginxService = &Service{
		Tag:         "nginx",
		Name:        "Nginx",
		UpgradeVars: []string{"nginx_package_state=latest"},
	}
	phpService = &Service{
		Tag:           "php",
		Name:          "PHP",
		VersionVar:    "php_version",
		UpgradeVars:   []string{"apt_package_state=latest"},
		LatestRelease: phpNet.latest,
	}
	mariaDBService = &Service{
		Tag:         "mariadb",
		Name:        "MariaDB",
		VersionVar:  "mariadb_version",
		UpgradeVars: []string{"mariadb_server_package_state=latest", "mariadb_client_package_state=latest"},
		UpgradeNote: "Back up your databases before upgrading MariaDB",
	}
)

// services are all services, in the order a fix lists them.
var services = []*Service{nginxService, phpService, mariaDBService}
