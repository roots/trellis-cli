package config

import (
	"errors"
	"fmt"
	"net"
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	_ "time/tzdata" // validate time zones the same way on every OS
)

const DevelopmentEnv = "development"

func applyDefaults(e *Environment) {
	s := &e.Server
	if s.Swap == "" {
		s.Swap = "auto"
	}
	if s.OS == "" {
		s.OS = "ubuntu-24.04"
	}
	if s.AutoUpdates == "" {
		s.AutoUpdates = UpdatesAll
	}
	if s.Timezone == "" {
		s.Timezone = "Etc/UTC"
	}
	if s.PHP.Version == "" {
		s.PHP.Version = "8.4"
	}
	f := &s.PHP.FPM
	if f.PM == "" {
		f.PM = "dynamic"
	}
	if f.MaxChildren == 0 {
		f.MaxChildren = 10
	}
	if f.StartServers == 0 {
		f.StartServers = 1
	}
	if f.MinSpareServers == 0 {
		f.MinSpareServers = 1
	}
	if f.MaxSpareServers == 0 {
		f.MaxSpareServers = 3
	}
	if f.MaxRequests == 0 {
		f.MaxRequests = 500
	}
	if len(s.SSH.AllowFrom) == 0 {
		s.SSH.AllowFrom = []string{"any"}
	}
	if s.SSH.RateLimit == nil {
		s.SSH.RateLimit = ptr(true)
	}
	if s.Fail2ban.WPLogin == nil {
		s.Fail2ban.WPLogin = ptr(true)
	}

	if e.Port == 0 {
		e.Port = 22
	}
	if e.Database != nil && e.Database.Port == 0 {
		e.Database.Port = 3306
	}

	for i := range e.Sites {
		site := &e.Sites[i]
		if site.Deploy.Keep == 0 {
			site.Deploy.Keep = 5
		}
		if site.Branch == "" {
			site.Branch = "main"
		}
		if site.Source == "" {
			site.Source = "../site"
		}
		if site.HTTPS.Mode == "" {
			site.HTTPS.Mode = HTTPSLetsEncrypt
			if e.Name == DevelopmentEnv {
				site.HTTPS.Mode = HTTPSSelfSigned
			}
		}
		if site.ObjectCache == "" {
			site.ObjectCache = Off
		}
		if site.Writable == nil {
			site.Writable = []string{"web/app/uploads"}
		}
		site.PHP.FPM = site.PHP.FPM.Or(s.PHP.FPM)
	}
}

func ptr[T any](v T) *T { return &v }

var (
	phpVersionRe = regexp.MustCompile(`^\d+\.\d+$`)
	unitNameRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	domainRe     = regexp.MustCompile(`^(\*\.)?([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)+[a-z0-9-]+$|^localhost$`)
	iniKeyRe     = regexp.MustCompile(`^[a-zA-Z0-9_.]+$`)
	envKeyRe     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	packageRe    = regexp.MustCompile(`^[a-z0-9][a-z0-9+.-]+$`)
	mariadbKeyRe = regexp.MustCompile(`^[a-z0-9_-]+$`)
)

func oneOf(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

var fpmModes = []string{"dynamic", "static", "ondemand"}

var (
	swapSizeRe = regexp.MustCompile(`^[1-9][0-9]*[MG]$`)
	siteNameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
)

func validate(e *Environment) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	p := "environments." + e.Name

	s := e.Server
	if !phpVersionRe.MatchString(s.PHP.Version) {
		add("%s: php.version %q must look like 8.4", p, s.PHP.Version)
	}
	if _, ok := LookupOS(s.OS); !ok {
		add("%s: os %q is not supported (supported: %s)", p, s.OS, strings.Join(SupportedOS(), ", "))
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "Local" {
		add("%s: timezone %q is not a known time zone (e.g. Etc/UTC, America/Toronto)", p, s.Timezone)
	}
	if !slices.Contains([]Choice{UpdatesAll, UpdatesDistro, UpdatesOff}, s.AutoUpdates) {
		add("%s: auto_updates must be %s, %s or false", p, UpdatesAll, UpdatesDistro)
	}
	if s.Swap != "auto" && s.Swap != Off && !swapSizeRe.MatchString(string(s.Swap)) {
		add("%s: swap must be auto, false or a size like 1G or 512M", p)
	}
	if !oneOf(s.PHP.FPM.PM, fpmModes...) {
		add("%s: php.fpm.pm must be dynamic, static or ondemand", p)
	}
	for k := range s.PHP.INI {
		if !iniKeyRe.MatchString(k) {
			add("%s: php.ini key %q is invalid", p, k)
		}
	}
	for k := range s.MariaDB {
		if !mariadbKeyRe.MatchString(k) {
			add("%s: mariadb key %q is invalid", p, k)
		}
	}
	for _, a := range s.SSH.AllowFrom {
		if a == "any" {
			continue
		}
		if _, _, err := net.ParseCIDR(a); err != nil && net.ParseIP(a) == nil {
			add("%s: ssh.allow_from %q must be an IP, CIDR or \"any\"", p, a)
		}
	}
	for _, a := range s.Firewall.Allow {
		if _, err := ParsePort(a); err != nil {
			add("%s: firewall.allow: %v", p, err)
		}
	}
	for _, pkg := range s.Packages {
		if !packageRe.MatchString(pkg) {
			add("%s: packages: %q is not a valid package name", p, pkg)
		}
	}
	cmdNames := map[string]bool{}
	for i, c := range s.Commands {
		where := fmt.Sprintf("%s: commands[%d]", p, i)
		if strings.TrimSpace(c.Run) == "" {
			add("%s: run is required", where)
			continue
		}
		name := c.CommandName()
		if !unitNameRe.MatchString(name) {
			add("%s: name %q must match %s", where, name, unitNameRe)
		}
		if cmdNames[name] {
			add("%s: another command is named %q; give one a name", where, name)
		}
		cmdNames[name] = true
		if strings.Contains(c.Run, "secret:") {
			add("%s: pass secrets through env (env: {KEY: secret:<path>}), not in run", where)
		}
		for _, o := range c.On {
			if !path.IsAbs(o) || path.Clean(o) != strings.TrimSuffix(o, "/") && o != "/" {
				add("%s: on: %q must be an absolute, clean server path", where, o)
			}
		}
		for k := range c.Env {
			if !envKeyRe.MatchString(k) {
				add("%s: env: invalid variable name %q", where, k)
			}
		}
	}
	if s.Mail != nil && s.Mail.SMTP == "" {
		add("%s: mail.smtp is required when mail is set", p)
	}

	if e.Name != DevelopmentEnv && len(e.Access.Admins) == 0 {
		add("%s: access.admins must list at least one key (you would be locked out)", p)
	}

	validateServer(e, add)
	validateSites(e, add)
	return errors.Join(errs...)
}

func validateServer(e *Environment, add func(string, ...any)) {
	p := "environments." + e.Name
	if e.Host == "" && e.Name != DevelopmentEnv {
		add("%s: host is required", p)
	}
	if e.Database != nil {
		if e.Database.Host == "" {
			add("%s: database.host is required", p)
		}
	}
}

func validateSites(e *Environment, add func(string, ...any)) {
	p := "environments." + e.Name
	if len(e.Sites) == 0 {
		add("%s: no sites", p)
	}
	domains := map[string]string{}
	dbNames, dbUsers := map[string]string{}, map[string]string{}
	for _, s := range e.Sites {
		sp := fmt.Sprintf("%s.sites.%s", p, s.Name)
		// Site names become paths and file names on the server
		// (/srv/www/<site>, nginx and PHP-FPM configs).
		if !siteNameRe.MatchString(s.Name) {
			add("%s: site name must be lowercase letters, digits, dots and hyphens (usually the domain)", sp)
		}
		if s.Deploy.Keep < 1 {
			add("%s: deploy.keep must be at least 1", sp)
		}
		for _, pat := range s.Deploy.Exclude {
			if _, err := path.Match(strings.Trim(pat, "/"), ""); err != nil || strings.Trim(pat, "/") == "" {
				add("%s: deploy.exclude pattern %q is invalid", sp, pat)
			}
		}
		if s.Domain == "" {
			add("%s: domain is required", sp)
		}
		for _, d := range s.Domains() {
			if d == "" {
				continue
			}
			if !domainRe.MatchString(d) {
				add("%s: %q is not a valid domain", sp, d)
			}
			if other, ok := domains[d]; ok {
				add("%s: domain %q is also used by %s", sp, d, other)
			}
			domains[d] = s.Name
		}
		if other, ok := dbNames[s.DBName()]; ok {
			add("%s: database name %q collides with %s; set db.name", sp, s.DBName(), other)
		}
		dbNames[s.DBName()] = s.Name
		if other, ok := dbUsers[s.DBUser()]; ok {
			add("%s: database user %q collides with %s; set db.user", sp, s.DBUser(), other)
		}
		dbUsers[s.DBUser()] = s.Name
		if !oneOf(s.PHP.FPM.PM, fpmModes...) {
			add("%s: php.fpm.pm must be dynamic, static or ondemand", sp)
		}
		if h := s.HTTPS; h.Mode == HTTPSCustom {
			if !strings.HasPrefix(h.Certificate, "file:") {
				add("%s: https.certificate must be file:<path> (a PEM file in the project, with any intermediates)", sp)
			}
			if !strings.HasPrefix(h.Key, "secret:") {
				add("%s: https.key must be secret:<path>: private keys belong in the encrypted secrets", sp)
			}
		}
		if !slices.Contains([]Choice{Off, ObjectCacheRedis, ObjectCacheMemcached}, s.ObjectCache) {
			add("%s: object_cache must be redis, memcached or false", sp)
		}
		for k := range s.Env {
			if !envKeyRe.MatchString(k) {
				add("%s: env: invalid variable name %q", sp, k)
			}
		}
		for k := range s.PHP.INI {
			if !iniKeyRe.MatchString(k) {
				add("%s: php.ini key %q is invalid", sp, k)
			}
		}
		for _, w := range s.Writable {
			if strings.HasPrefix(w, "/") || strings.Contains(w, "..") {
				add("%s: writable path %q must be relative and inside the release", sp, w)
			}
		}
	}
}
