package config

import (
	"reflect"
	"strings"
)

// SchemaID is where the published schema lives.
const SchemaID = "https://raw.githubusercontent.com/roots/trellis-cli/converge-engine/pkg/converge/config/trellis.schema.json"

// Schema returns a JSON Schema (draft 2020-12) for trellis.yml. It is
// generated from the config types, so it can't drift from what the loader
// accepts; descriptions and allowed values come from the tables below.
// Editors (yaml-language-server) and agents use it to write valid config.
func Schema() map[string]any {
	g := &schemaGen{defs: map[string]any{}}
	root := g.object(reflect.TypeOf(File{}))
	root["$schema"] = "https://json-schema.org/draft/2020-12/schema"
	root["$id"] = SchemaID
	root["title"] = "trellis.yml"
	root["description"] = "Trellis project config: server settings (defaults), access, sites and environments. " +
		"Merge rule: defaults → environments.<env>, sites.<name> → environments.<env>.sites.<name>; " +
		"maps merge, lists replace, null removes. Values like `secret:<path>` come from secrets/<env>.yml(.age)."
	root["required"] = []string{"version"}
	root["$defs"] = g.defs
	return root
}

type schemaGen struct{ defs map[string]any }

// descriptions are keyed by "<Type>.<yaml key>".
var schemaDescriptions = map[string]string{
	"File.version":      "Config format version. Must be 1.",
	"File.defaults":     "Server settings shared by every environment (each environment can override them).",
	"File.access":       "Who can log in and decrypt secrets, for every environment.",
	"File.sites":        "Sites, keyed by name (usually the domain). Environments override per site.",
	"File.environments": "Environments, e.g. development, staging, production. One server each.",

	"Environment.host":     "The server's address (one server per environment). Optional in development (use --lima or --host).",
	"Environment.user":     "SSH user for provisioning (default: root). Any other user needs passwordless sudo.",
	"Environment.port":     "SSH port (default 22).",
	"Environment.database": "An external database (e.g. RDS). Without it, MariaDB runs on the server.",
	"Environment.access":   "Overrides top-level access for this environment.",
	"Environment.sites":    "Per-environment site overrides, keyed by site name. null excludes a site from this environment.",

	"Server.os":           "The server's operating system (default ubuntu-24.04). Plans refuse to run on a server with a different OS.",
	"Server.timezone":     "IANA time zone, e.g. Etc/UTC or America/Toronto.",
	"Server.swap":         "auto (a swapfile of min(RAM, 2G) if the server has no swap), a size like 1G, or false.",
	"Server.auto_updates": "What unattended-upgrades installs daily (around 6:00 server time): all (default: the distro's security and bugfix updates, plus bugfix and security releases of nginx, PHP and MariaDB within its series), distro (the distro's default: its security updates only) or false (nothing automatic; `trellis converge upgrade` installs what all would).",
	"Server.php":          "PHP version, php.ini settings and PHP-FPM pool settings.",
	"Server.mariadb":      "MariaDB [mysqld] settings (e.g. innodb_buffer_pool_size: 512M).",
	"Server.mail":         "SMTP relay for outgoing mail (msmtp).",
	"Server.ssh":          "Who can reach SSH.",
	"Server.fail2ban":     "WordPress fail2ban jails (sshd's is always on).",
	"Server.packages":     "Extra apt packages to install.",
	"Server.commands":     "Escape hatch: shell commands run as root, last, re-run when run, env or a watched file (on) changes. Files go in server/files/.",

	"PHP.version": "PHP version from packages.sury.org, e.g. \"8.4\".",
	"PHP.ini":     "php.ini settings for PHP-FPM and the CLI.",
	"PHP.fpm":     "PHP-FPM process manager settings for every site's pool (each site can override them under its php.fpm).",

	"SSH.allow_from": "IPs or CIDRs allowed to SSH, or [any].",
	"SSH.rate_limit": "Rate-limit new SSH connections per source IP (default true).",

	"Fail2ban.wp_login": "Ban IPs that keep failing WordPress logins (default true). Turn it off behind a proxy nginx doesn't see real client IPs through. The xmlrpc.php jail is on when any site allows XML-RPC.",

	"Mail.smtp":     "SMTP server as host:port, e.g. smtp.postmarkapp.com:587.",
	"Mail.user":     "SMTP user (may be secret:<path>).",
	"Mail.password": "SMTP password (use secret:<path>).",
	"Mail.from":     "From address for all mail sent by the server.",

	"Access.admins":     "Admins log in as admin (passwordless sudo) or root, and can decrypt secrets. github:<user>, file:<path> or a public key.",
	"Access.deploy":     "Keys that can log in as the web user (default: admins).",
	"Access.recipients": "Extra age public keys (age1…) that can decrypt secrets, e.g. CI.",

	"ExternalDatabase.host":  "Database server hostname.",
	"ExternalDatabase.port":  "Database port (default 3306).",
	"ExternalDatabase.tls":   "Connect over TLS (default true; false for a database on a private network without TLS).",
	"ExternalDatabase.admin": "Admin credentials used to create databases and users.",

	"Site.repo":         "Git repository (used for the site's identity; deploys build from local_path).",
	"Site.path":         "Subdirectory of the repo containing the Bedrock site (repo subtree).",
	"Site.branch":       "Branch deployed by default (default main).",
	"Site.local_path":   "Local clone of the site, relative to trellis.yml. Deploys build from it.",
	"Site.domain":       "Canonical domain.",
	"Site.redirects":    "Domains redirected to the canonical domain.",
	"Site.https":        "HTTPS (default true: an automatic Let's Encrypt certificate, self-signed in development). false serves plain HTTP; self-signed; or the site's own certificate: {certificate: file:<path>, key: secret:<path>}.",
	"Site.cache":        "nginx full-page cache (FastCGI; default false).",
	"Site.object_cache": "redis, memcached or false (default).",
	"Site.xmlrpc":       "Allow xmlrpc.php (default false: blocked).",
	"Site.multisite":    "false (default), true (subdirectories), subdomains or subdirectories.",
	"Site.writable":     "Release paths shared between releases (default [web/app/uploads]).",
	"Site.php":          "Per-site php.ini and PHP-FPM settings (the site's pool).",
	"Site.env":          "Extra .env variables (values may be secret:<path>).",
	"Site.db":           "Database name and user (default <site>_<env> and <site>).",
	"Site.deploy":       "Deploy settings.",

	"SiteDB.name": "Database name (default <site>_<env>).",
	"SiteDB.user": "Database user (default <site>).",

	"SitePHP.ini": "php.ini settings for this site's PHP-FPM pool.",
	"SitePHP.fpm": "PHP-FPM settings for this site's pool; unset fields come from the server's php.fpm.",

	"FPM.pm":                "Process manager: dynamic (default), static or ondemand.",
	"FPM.max_children":      "Maximum PHP-FPM workers per site (default 10).",
	"FPM.start_servers":     "Workers started with the pool (dynamic).",
	"FPM.min_spare_servers": "Minimum idle workers (dynamic).",
	"FPM.max_spare_servers": "Maximum idle workers (dynamic).",
	"FPM.max_requests":      "Requests a worker serves before being recycled (default 500).",

	"Command.name": "Name shown in plans and history (default: derived from run). Letters, digits, - and _.",
	"Command.run":  "Shell command, run as root with /bin/sh -c after everything else. Runs once, then when run, env or a watched file changes.",
	"Command.on":   "Server paths (files, or directories ending in /) managed by Trellis or server/files/; a change re-runs the command.",
	"Command.env":  "Environment variables (values may be secret:<path>; kept out of plans and history).",

	"DBAdmin.user":     "Admin user on the external database.",
	"DBAdmin.password": "Admin password (use secret:<path>).",

	"SiteDeploy.keep":      "Releases kept on the server (default 5).",
	"SiteDeploy.update_db": "Run wp core update-db after the switch (default true).",
	"SiteDeploy.exclude":   "Paths kept out of the release: a pattern without a slash matches names (*.map), one with a slash matches from the release root.",
}

var schemaEnums = map[string][]any{
	"Site.object_cache":   {ObjectCacheRedis, ObjectCacheMemcached},
	"FPM.pm":              anys(fpmModes),
	"Server.os":           anys(SupportedOS()),
	"Server.auto_updates": {UpdatesAll, UpdatesDistro},
	"File.version":        {1},
}

var schemaPatterns = map[string]string{
	"Server.swap": `^(auto|[1-9][0-9]*[MG])$`,
	"PHP.version": `^[0-9]+\.[0-9]+$`,
}

func yamlKey(f reflect.StructField) (key string, inline, skip bool) {
	tag := f.Tag.Get("yaml")
	name, opts, _ := strings.Cut(tag, ",")
	if name == "-" || !f.IsExported() {
		return "", false, true
	}
	if strings.Contains(opts, "inline") {
		return "", true, false
	}
	if name == "" {
		name = strings.ToLower(f.Name)
	}
	return name, false, false
}

// scalarish is what a map[string]string value accepts in YAML.
var scalarish = map[string]any{"type": []string{"string", "number", "boolean"}}

func (g *schemaGen) typ(t reflect.Type, owner, key string) map[string]any {
	if t == reflect.TypeOf(HTTPS{}) {
		return map[string]any{"description": schemaDescriptions[owner+"."+key], "oneOf": []any{
			map[string]any{"type": "boolean"},
			map[string]any{"const": HTTPSSelfSigned},
			map[string]any{"type": "object", "additionalProperties": false, "required": []string{"certificate", "key"},
				"properties": map[string]any{
					"certificate": map[string]any{"type": "string", "pattern": "^file:", "description": "PEM certificate (with intermediates) in the project: file:<path>."},
					"key":         map[string]any{"type": "string", "pattern": "^secret:", "description": "Private key: secret:<path>."},
				}},
		}}
	}
	if t == reflect.TypeOf(Multisite{}) {
		return map[string]any{"description": schemaDescriptions[owner+"."+key], "oneOf": []any{
			map[string]any{"type": "boolean"},
			map[string]any{"enum": []string{"subdomains", "subdirectories"}},
		}}
	}
	if t == reflect.TypeOf(Choice("")) {
		// One of the options (enum or pattern below), or false.
		str := map[string]any{"type": "string"}
		if e, ok := schemaEnums[owner+"."+key]; ok {
			str["enum"] = e
		}
		if p, ok := schemaPatterns[owner+"."+key]; ok {
			str["pattern"] = p
		}
		return map[string]any{"description": schemaDescriptions[owner+"."+key], "anyOf": []any{map[string]any{"const": false}, str}}
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	var s map[string]any
	switch t.Kind() {
	case reflect.String:
		s = map[string]any{"type": "string"}
	case reflect.Bool:
		s = map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int64:
		s = map[string]any{"type": "integer"}
	case reflect.Slice:
		s = map[string]any{"type": "array", "items": g.typ(t.Elem(), "", "")}
	case reflect.Map:
		if t.Elem().Kind() == reflect.String {
			s = map[string]any{"type": "object", "additionalProperties": scalarish}
		} else {
			s = map[string]any{"type": "object", "additionalProperties": g.typ(t.Elem(), "", "")}
		}
	case reflect.Struct:
		name := t.Name()
		if _, ok := g.defs[name]; !ok {
			g.defs[name] = map[string]any{} // break cycles
			g.defs[name] = g.object(t)
		}
		s = map[string]any{"$ref": "#/$defs/" + name}
	default:
		s = map[string]any{}
	}
	if key != "" {
		id := owner + "." + key
		if d, ok := schemaDescriptions[id]; ok {
			s["description"] = d
		}
		if e, ok := schemaEnums[id]; ok {
			s["enum"] = e
		}
		if p, ok := schemaPatterns[id]; ok {
			s["pattern"] = p
		}
	}
	return s
}

func (g *schemaGen) object(t reflect.Type) map[string]any {
	props := map[string]any{}
	g.fields(t, t.Name(), props)
	return map[string]any{"type": "object", "additionalProperties": false, "properties": props}
}

func (g *schemaGen) fields(t reflect.Type, owner string, props map[string]any) {
	for i := range t.NumField() {
		f := t.Field(i)
		key, inline, skip := yamlKey(f)
		if skip {
			continue
		}
		if inline {
			g.fields(f.Type, f.Type.Name(), props)
			continue
		}
		s := g.typ(f.Type, owner, key)
		if t == reflect.TypeOf(Environment{}) && key == "sites" {
			// Per-environment overrides; null excludes the site.
			s = map[string]any{"type": "object", "description": s["description"],
				"propertyNames":        map[string]any{"pattern": siteNameRe.String()},
				"additionalProperties": map[string]any{"anyOf": []any{map[string]any{"$ref": "#/$defs/Site"}, map[string]any{"type": "null"}}}}
		}
		if t == reflect.TypeOf(File{}) && key == "sites" {
			s["propertyNames"] = map[string]any{"pattern": siteNameRe.String()}
		}
		props[key] = s
	}
}

func anys[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}
