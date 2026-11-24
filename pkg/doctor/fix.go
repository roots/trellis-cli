package doctor

import (
	"fmt"
	"slices"
	"strings"
)

// Fix is the provisioning command that brings an environment's outdated
// services up to date.
type Fix struct {
	Services []*Service
	// Command is the full command line, for display.
	Command string
	Notes   []string

	env string
}

func (f *Fix) tags() string {
	var tags []string
	for _, s := range f.Services {
		tags = append(tags, s.Tag)
	}
	return strings.Join(tags, ",")
}

func (f *Fix) extraVars() string {
	var vars []string
	for _, s := range f.Services {
		vars = append(vars, s.UpgradeVars...)
	}
	return strings.Join(vars, " ")
}

// ProvisionArgs are the `trellis provision` arguments that apply the fix.
func (f *Fix) ProvisionArgs() []string {
	return []string{"--tags", f.tags(), "--extra-vars", f.extraVars(), f.env}
}

// envFix returns the fix for an environment's hosts. Provisioning runs on
// every host in an environment, so a service is left out if any host withheld
// it, and there's no fix when a host couldn't be checked. note explains a
// missing fix when there would otherwise have been one.
func envFix(env string, hosts []HostResult) (fix *Fix, note string) {
	var upgrades, withheld []*Service
	stale, unchecked := false, false
	for _, h := range hosts {
		upgrades = append(upgrades, h.upgrades...)
		withheld = append(withheld, h.withheld...)
		stale = stale || h.staleLists
		unchecked = unchecked || h.Error != ""
	}

	upgrades = slices.DeleteFunc(upgrades, func(s *Service) bool { return slices.Contains(withheld, s) })

	if len(upgrades) > 0 && unchecked {
		return nil, "not every host could be checked, and provisioning runs on all of them"
	}

	fix = newFix(env, upgrades)
	if fix != nil && stale {
		fix.Notes = append([]string{"The server's package lists are out of date, so this may include packages that are already current; provisioning refreshes them first"}, fix.Notes...)
	}

	return fix, ""
}

// newFix returns the command that upgrades services in an environment, or
// nil if there's nothing to upgrade.
func newFix(env string, upgrades []*Service) *Fix {
	fix := &Fix{env: env}

	for _, s := range services {
		if slices.Contains(upgrades, s) {
			fix.Services = append(fix.Services, s)
			if s.UpgradeNote != "" {
				fix.Notes = append(fix.Notes, s.UpgradeNote)
			}
		}
	}

	if len(fix.Services) == 0 {
		return nil
	}

	fix.Command = fmt.Sprintf("trellis provision --tags %s --extra-vars \"%s\" %s", fix.tags(), fix.extraVars(), env)

	return fix
}
