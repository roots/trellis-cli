package compile

import (
	"maps"
	"strconv"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/lock"
	m "github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/secrets"
	"github.com/roots/trellis-cli/pkg/converge/templates"
)

// nginxFingerprints are the keys in https://nginx.org/keys/nginx_signing.key
// (as published on nginx.org/en/linux_packages.html).
var nginxFingerprints = []string{
	"8540A6F18833A80E9C1653A42FD21310B49F6B46",
	"573BFD6B3D8FBC641079A6ABABF5BD827BD9BF62",
	"9E9BE90EACBCDE69FE9B204CBCDCD8A38D88A2B3",
}

const (
	defaultWPCLIVersion    = "2.12.0"
	defaultComposerVersion = "latest-2.x"
	suryFingerprint        = "15058500A0235D97F5D10063B188E2B695BD4743"
	mariadbFingerprint     = "177F4010FE56CA3336300305F1656F24C74CD1D8"
)

// phpINIDefaults mirrors Trellis's php role defaults.
var phpINIDefaults = map[string]string{
	"error_reporting":                   "E_ALL & ~E_DEPRECATED & ~E_STRICT",
	"display_errors":                    "Off",
	"display_startup_errors":            "Off",
	"max_execution_time":                "120",
	"max_input_time":                    "300",
	"max_input_vars":                    "1000",
	"memory_limit":                      "96M",
	"mysqlnd.collect_memory_statistics": "Off",
	"post_max_size":                     "25M",
	"upload_max_filesize":               "25M",
	"session.cookie_httponly":           "On",
	"session.cookie_secure":             "Off",
	"session.upload_progress.enabled":   "Off",
	"output_buffering":                  "Off",
	"expose_php":                        "Off",
	"opcache.enable":                    "1",
	"opcache.enable_cli":                "1",
	"opcache.enable_file_override":      "0",
	"opcache.interned_strings_buffer":   "8",
	"opcache.max_accelerated_files":     "4000",
	"opcache.memory_consumption":        "128",
	"opcache.revalidate_freq":           "60",
	"opcache.validate_timestamps":       "1",
	"opcache.max_wasted_percentage":     "5",
	"opcache.huge_code_pages":           "0",
	"opcache.jit":                       "disable",
	"opcache.jit_buffer_size":           "0",
}

func (b *builder) phpVersion() string { return b.env.Server.PHP.Version }
func (b *builder) phpUnit() string    { return "php" + b.phpVersion() + "-fpm.service" }
func (b *builder) phpReload() m.Notify {
	return m.ReloadUnit(b.phpUnit(), "php-fpm"+b.phpVersion(), "-t")
}

func (b *builder) phpINI() map[string]string {
	ini := map[string]string{}
	maps.Copy(ini, phpINIDefaults)
	ini["date.timezone"] = b.env.Server.Timezone
	if b.env.Server.Mail != nil {
		ini["sendmail_path"] = "/usr/bin/msmtp -t"
	}
	maps.Copy(ini, b.env.Server.PHP.INI)
	return ini
}

func (b *builder) webStack() {
	v := b.phpVersion()
	b.repo(aptRepo{
		name: "nginx", uri: "https://nginx.org/packages/mainline/" + b.distro, suite: b.codename, components: "nginx",
		keyURL: "https://nginx.org/keys/nginx_signing.key", keyFile: "nginx.asc", fingerprints: nginxFingerprints,
	})
	b.repo(aptRepo{
		name: "sury-php", uri: "https://packages.sury.org/php/", suite: b.codename, components: "main",
		keyURL: "https://packages.sury.org/php/apt.gpg", keyFile: "sury-php.gpg", fingerprints: []string{suryFingerprint},
	})

	b.pkg("nginx")
	b.add(m.NewRootFile("/etc/logrotate.d/trellis-sites", siteLogrotate, 0o644))
	for _, ext := range []string{"bcmath", "cli", "curl", "fpm", "imagick", "intl", "mbstring", "mysql", "xml", "zip"} {
		b.pkg("php" + v + "-" + ext)
	}
	if !b.env.LocalDB() {
		b.pkg("mariadb-client")
	}

	fastcgi := false
	anySSL := false
	letsencrypt := false
	for _, s := range b.env.Sites {
		fastcgi = fastcgi || s.Cache == config.CacheFastCGI
		anySSL = anySSL || s.SSL != config.SSLNone
		letsencrypt = letsencrypt || s.SSL == config.SSLLetsEncrypt
	}

	// nginx
	b.add(
		m.NewRootFile("/etc/nginx/nginx.conf", b.render("nginx/nginx.conf", map[string]any{"FastCGICache": fastcgi}), 0o644, nginxReload),
		m.NewRootFile("/etc/nginx/sites/00-no-default.conf", b.render("nginx/no-default.conf", map[string]any{"AnySSL": anySSL}), 0o644, nginxReload),
		m.NewAbsentFile("/etc/nginx/conf.d/default.conf", nginxReload),
		m.NewService("nginx.service", enabled, true),
	)

	// PHP
	ini := b.phpINI()
	cli := maps.Clone(ini)
	cli["memory_limit"] = "-1"
	b.add(
		m.NewRootFile("/etc/php/"+v+"/fpm/conf.d/90-trellis.ini", b.render("php/php.ini", map[string]any{"INI": templates.Sorted(ini)}), 0o644, b.phpReload()),
		m.NewRootFile("/etc/php/"+v+"/cli/conf.d/90-trellis.ini", b.render("php/php.ini", map[string]any{"INI": templates.Sorted(cli)}), 0o644),
		m.NewAbsentFile("/etc/php/"+v+"/fpm/pool.d/www.conf", b.phpReload()),
		m.NewRootFile("/etc/php/"+v+"/fpm/php-fpm.conf", b.render("php/php-fpm.conf", map[string]any{"Version": v}), 0o644, b.phpReload()),
		m.NewService(b.phpUnit(), enabled, true),
	)

	b.tools()
	b.objectCaches()
	if letsencrypt {
		b.pkg("certbot")
		b.add(m.NewRootFile("/etc/letsencrypt/renewal-hooks/deploy/trellis", certbotDeployHook, 0o755))
	}
	for _, s := range b.env.Sites {
		b.site(s, ini)
	}
}

// certbotDeployHook installs renewed certificates where nginx expects them.
const certbotDeployHook = `#!/bin/sh
# Managed by Trellis: copy renewed certificates into /etc/nginx/ssl.
set -eu
name=$(basename "$RENEWED_LINEAGE")
install -m 0644 "$RENEWED_LINEAGE/fullchain.pem" "/etc/nginx/ssl/$name.crt"
install -m 0600 "$RENEWED_LINEAGE/privkey.pem" "/etc/nginx/ssl/$name.key"
systemctl reload nginx.service
`

func (b *builder) tools() {
	wp := b.tool("wp-cli", defaultWPCLIVersion)
	composer := b.tool("composer", defaultComposerVersion)
	b.add(
		m.NewDownload("/usr/local/bin/wp", wp.url, 0o755).WithSHA256(wp.sha),
		m.NewDownload("/usr/local/bin/composer", composer.url, 0o755).WithSHA256(composer.sha),
	)
}

type toolRef struct{ url, sha string }

func (b *builder) tool(name, def string) toolRef {
	t, ok := b.in.Lock.Tools[name]
	if !ok {
		t.Version = def
	}
	if t.SHA256 == "" {
		b.warnf("%s %s is not pinned in trellis.lock (run `trellis converge lock`)", name, t.Version)
	}
	return toolRef{url: lock.ToolURL(name, t.Version), sha: t.SHA256}
}

func (b *builder) objectCaches() {
	v := b.phpVersion()
	redis, memcached := false, false
	for _, s := range b.env.Sites {
		redis = redis || s.ObjectCache == config.ObjectCacheRedis
		memcached = memcached || s.ObjectCache == config.ObjectCacheMemcached
	}
	if redis {
		b.pkg("redis-server", "php"+v+"-redis")
		b.add(m.NewService("redis-server.service", enabled, true))
	}
	if memcached {
		b.pkg("memcached", "php"+v+"-memcached")
		b.add(m.NewService("memcached.service", enabled, true))
	}
}

func (b *builder) site(s config.Site, ini map[string]string) {
	v := b.phpVersion()
	root := "/srv/www/" + s.Name
	ssl := s.SSL != config.SSLNone
	certPath, keyPath := "/etc/nginx/ssl/"+s.Name+".crt", "/etc/nginx/ssl/"+s.Name+".key"

	postMax := ini["post_max_size"]
	if siteMax, ok := s.PHP.INI["post_max_size"]; ok {
		postMax = siteMax
	}
	conf := b.render("nginx/site.conf", map[string]any{
		"Site":              s,
		"SSL":               ssl,
		"SelfSigned":        s.SSL == config.SSLSelfSigned,
		"Scheme":            scheme(ssl),
		"CertPath":          certPath,
		"KeyPath":           keyPath,
		"FastCGICache":      s.Cache == config.CacheFastCGI,
		"Subdomains":        s.Multisite.Enabled && s.Multisite.Mode == "subdomains",
		"Development":       b.env.Name == config.DevelopmentEnv,
		"ClientMaxBodySize": strings.ToLower(postMax),
	})
	b.add(
		m.NewRootFile("/etc/nginx/sites/"+s.Name+".conf", conf, 0o644, nginxReload),
		m.NewRootFile("/etc/php/"+v+"/fpm/pool.d/"+s.Name+".conf", b.render("php/pool.conf", map[string]any{
			"Site": s, "FPM": b.env.Server.PHP.FPM, "INI": templates.Sorted(s.PHP.INI),
		}), 0o644, b.phpReload()),
		m.NewFile(root+"/shared/.env", b.dotenv(s), 0o640, "web", "www-data").AsSensitive(),
	)

	if ssl {
		provider := m.CertSelfSigned
		email := ""
		if s.SSL == config.SSLLetsEncrypt {
			provider = m.CertLetsEncrypt
			if b.env.Server.Mail != nil {
				email = b.env.Server.Mail.From
			}
			if s.Multisite.Mode == "subdomains" {
				b.warnf("%s: Let's Encrypt HTTP-01 can't issue wildcard certificates; subdomain sites need their own domains listed", s.Name)
			}
		}
		cert := m.NewCert(s.Name, provider, s.Domains(), certPath, keyPath, nginxReload)
		cert.Cert.Email = email
		b.add(cert)
	}

	// WP-Cron via a systemd timer instead of cron or page loads.
	unit := "wp-cron-" + unitSafe(s.Name)
	data := map[string]any{"Site": s}
	b.add(
		m.NewRootFile("/etc/systemd/system/"+unit+".service", b.render("systemd/wp-cron.service", data), 0o644, daemonReload),
		m.NewRootFile("/etc/systemd/system/"+unit+".timer", b.render("systemd/wp-cron.timer", data), 0o644, daemonReload, m.RestartUnit(unit+".timer")),
		m.NewService(unit+".timer", enabled, true),
	)
}

func scheme(ssl bool) string {
	if ssl {
		return "https"
	}
	return "http"
}

// unitSafe maps a site name to a systemd unit name fragment.
func unitSafe(s string) string {
	return strings.NewReplacer(".", "-", "_", "-").Replace(s)
}

func (b *builder) dbHost() string {
	if db := b.env.Database; db != nil {
		return db.Host + ":" + strconv.Itoa(db.Port)
	}
	return "localhost"
}

func (b *builder) dotenv(s config.Site) string {
	home := scheme(s.SSL != config.SSLNone) + "://" + s.Domain
	vars := map[string]string{
		"DB_NAME":         s.DBName(),
		"DB_USER":         s.DBUser(),
		"DB_PASSWORD":     b.required("sites", s.Name, "db_password"),
		"DB_HOST":         b.dbHost(),
		"WP_ENV":          b.env.Name,
		"WP_HOME":         home,
		"WP_SITEURL":      home + "/wp",
		"DISABLE_WP_CRON": "true",
	}
	if s.Multisite.Enabled {
		vars["DOMAIN_CURRENT_SITE"] = s.Domain
	}
	for _, k := range secrets.SaltKeys {
		vars[strings.ToUpper(k)] = b.required("sites", s.Name, "salts", k)
	}
	for k, v := range s.Env {
		vars[k] = b.secret(v, "sites."+s.Name+".env."+k)
	}
	kv := templates.Sorted(vars)
	for i := range kv {
		kv[i].Value = dotenvQuote(kv[i].Value)
	}
	return b.render("wordpress/env", map[string]any{"Vars": kv})
}

// dotenvQuote escapes a value for a single-quoted phpdotenv string.
func dotenvQuote(v string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(v)
}

// siteLogrotate rotates every site's nginx logs. nginx reopens its logs on
// USR1 to the master process.
const siteLogrotate = `# Managed by Trellis.
/srv/www/*/logs/*.log {
  daily
  rotate 14
  missingok
  notifempty
  compress
  delaycompress
  create 0640 web www-data
  sharedscripts
  postrotate
    systemctl kill --kill-whom=main --signal=USR1 nginx.service 2>/dev/null || true
  endscript
}
`
