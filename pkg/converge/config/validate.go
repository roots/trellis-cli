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
	if len(s.Security.SSH.AllowFrom) == 0 {
		s.Security.SSH.AllowFrom = []string{"any"}
	}
	if s.Security.SSH.RateLimit == nil {
		t := true
		s.Security.SSH.RateLimit = &t
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
		if site.SSL == "" {
			site.SSL = SSLLetsEncrypt
			if e.Name == DevelopmentEnv {
				site.SSL = SSLSelfSigned
			}
		}
		if site.Cache == "" {
			site.Cache = CacheOff
		}
		if site.ObjectCache == "" {
			site.ObjectCache = ObjectCacheNone
		}
		if site.Writable == nil {
			site.Writable = []string{"web/app/uploads"}
		}
	}
}

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
	if !slices.Contains(SupportedOS, s.OS) {
		add("%s: os %q is not supported (supported: %s)", p, s.OS, strings.Join(SupportedOS, ", "))
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "Local" {
		add("%s: timezone %q is not a known time zone (e.g. Etc/UTC, America/Toronto)", p, s.Timezone)
	}
	if s.Swap != "auto" && s.Swap != "off" && !swapSizeRe.MatchString(s.Swap) {
		add("%s: swap must be auto, off or a size like 1G or 512M", p)
	}
	if !oneOf(s.PHP.FPM.PM, "dynamic", "static", "ondemand") {
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
	for _, a := range s.Security.SSH.AllowFrom {
		if a == "any" {
			continue
		}
		if _, _, err := net.ParseCIDR(a); err != nil && net.ParseIP(a) == nil {
			add("%s: security.ssh.allow_from %q must be an IP, CIDR or \"any\"", p, a)
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
		if e.Database.TLS != "" && e.Database.TLS != "required" {
			add("%s: database.tls must be \"required\" or omitted", p)
		}
	}
}

func validateSites(e *Environment, add func(string, ...any)) {
	p := "environments." + e.Name
	if len(e.Sites) == 0 {
		add("%s: no sites", p)
	}
	domains := map[string]string{}
	dbNames := map[string]string{}
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
		if e.Name != DevelopmentEnv && s.Repo == "" {
			add("%s: repo is required", sp)
		}
		if !oneOf(s.SSL, SSLLetsEncrypt, SSLSelfSigned, SSLNone) {
			add("%s: ssl must be letsencrypt, self-signed or none", sp)
		}
		if !oneOf(s.Cache, CacheOff, CacheFastCGI) {
			add("%s: cache must be off or fastcgi", sp)
		}
		if !oneOf(s.ObjectCache, ObjectCacheNone, ObjectCacheRedis, ObjectCacheMemcached) {
			add("%s: object_cache must be none, redis or memcached", sp)
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
