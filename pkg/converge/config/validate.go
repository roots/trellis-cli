package config

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
)

const DevelopmentEnv = "development"

func applyDefaults(e *Environment) {
	s := &e.Server
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

	if len(e.Nodes) == 0 && e.Name == DevelopmentEnv {
		e.Nodes = []Node{{Name: DevelopmentEnv}}
	}
	for i := range e.Nodes {
		n := &e.Nodes[i]
		if n.Name == "" {
			n.Name = n.Host
			if n.Name == "" {
				n.Name = e.Name
			}
		}
		if len(n.Roles) == 0 {
			n.Roles = []string{RoleWeb, RoleDB}
			if e.Database != nil {
				n.Roles = []string{RoleWeb}
			}
		}
		if n.Port == 0 {
			n.Port = 22
		}
	}
	if e.Database != nil && e.Database.Port == 0 {
		e.Database.Port = 3306
	}

	for i := range e.Sites {
		site := &e.Sites[i]
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

func validate(e *Environment) error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	p := "environments." + e.Name

	s := e.Server
	if !phpVersionRe.MatchString(s.PHP.Version) {
		add("%s: php.version %q must look like 8.4", p, s.PHP.Version)
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
	for name, svc := range s.Services {
		if !unitNameRe.MatchString(name) {
			add("%s: services.%s: name must match %s", p, name, unitNameRe)
		}
		if svc.Image == "" {
			add("%s: services.%s: image is required", p, name)
		}
		for k := range svc.Env {
			if !envKeyRe.MatchString(k) {
				add("%s: services.%s.env: invalid variable name %q", p, name, k)
			}
		}
	}
	execNames := map[string]bool{}
	for i, x := range s.Exec {
		if !unitNameRe.MatchString(x.Name) {
			add("%s: exec[%d].name %q must match %s", p, i, x.Name, unitNameRe)
		}
		if execNames[x.Name] {
			add("%s: exec %q is declared twice", p, x.Name)
		}
		execNames[x.Name] = true
		if strings.TrimSpace(x.Run) == "" {
			add("%s: exec %q: run is required", p, x.Name)
		}
	}
	if s.Mail != nil && s.Mail.SMTP == "" {
		add("%s: mail.smtp is required when mail is set", p)
	}

	if e.Name != DevelopmentEnv && len(e.Access.Admins) == 0 {
		add("%s: access.admins must list at least one key (you would be locked out)", p)
	}

	validateNodes(e, add)
	validateSites(e, add)
	return errors.Join(errs...)
}

func validateNodes(e *Environment, add func(string, ...any)) {
	p := "environments." + e.Name
	if len(e.Nodes) == 0 {
		add("%s: at least one node is required", p)
		return
	}
	names := map[string]bool{}
	var web, db []Node
	for _, n := range e.Nodes {
		if names[n.Name] {
			add("%s: node name %q is used twice", p, n.Name)
		}
		names[n.Name] = true
		if n.Host == "" && e.Name != DevelopmentEnv {
			add("%s: node %q: host is required", p, n.Name)
		}
		for _, r := range n.Roles {
			if !oneOf(r, RoleWeb, RoleDB) {
				add("%s: node %q: unknown role %q (allowed: web, db)", p, n.Name, r)
			}
		}
		if n.PrivateIP != "" && net.ParseIP(n.PrivateIP) == nil {
			add("%s: node %q: private_ip %q is not an IP", p, n.Name, n.PrivateIP)
		}
		if n.HasRole(RoleWeb) {
			web = append(web, n)
		}
		if n.HasRole(RoleDB) {
			db = append(db, n)
		}
	}
	// The supported topologies: one all-in-one node, or one web + one db
	// node, or one web node + an external database. Nothing else.
	if len(web) != 1 {
		add("%s: exactly one node must have the web role (found %d)", p, len(web))
	}
	if e.Database != nil {
		if len(db) > 0 {
			add("%s: database is external, so no node may have the db role", p)
		}
		if e.Database.Host == "" {
			add("%s: database.host is required", p)
		}
		if e.Database.TLS != "" && e.Database.TLS != "required" {
			add("%s: database.tls must be \"required\" or omitted", p)
		}
	} else if len(db) != 1 {
		add("%s: exactly one node must have the db role, or set an external database (found %d)", p, len(db))
	}
	if len(web) == 1 && len(db) == 1 && web[0].Name != db[0].Name {
		for _, n := range []Node{web[0], db[0]} {
			if n.PrivateIP == "" {
				add("%s: node %q: private_ip is required when web and db are on separate nodes", p, n.Name)
			}
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
