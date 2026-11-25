package engine

import (
	"bufio"
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/host"
	"github.com/roots/trellis-cli/pkg/converge/manifest"
	"github.com/roots/trellis-cli/pkg/converge/state"
)

// This file holds the command builders and output parsers for the system
// tools the engine drives (dpkg, apt, systemctl, mariadb, gpg, openssl,
// certbot). They are pure, so they are unit tested without a host.

var aptEnv = []string{"DEBIAN_FRONTEND=noninteractive", "NEEDRESTART_MODE=a"}

func dpkgQueryCmd(names []string) host.Cmd {
	argv := append([]string{"dpkg-query", "-W", "-f", "${Package}\t${Version}\t${db:Status-Abbrev}\n", "--"}, names...)
	return host.Cmd{Argv: argv}
}

// parseDpkg returns installed packages. Lines for packages that are known
// but not installed (status other than "ii") are skipped.
func parseDpkg(out []byte) map[string]string {
	pkgs := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 3 {
			continue
		}
		if strings.HasPrefix(f[2], "ii") || strings.HasPrefix(f[2], "hi") {
			pkgs[f[0]] = f[1]
		}
	}
	return pkgs
}

func aptInstallCmd(pkgs []manifest.Package) host.Cmd {
	argv := []string{"apt-get", "install", "-y", "-q",
		"-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold"}
	argv = append(argv, "--")
	for _, p := range pkgs {
		if p.Version != "" {
			argv = append(argv, p.Name+"="+p.Version)
		} else {
			argv = append(argv, p.Name)
		}
	}
	return host.Cmd{Argv: argv, Env: aptEnv}
}

func aptRemoveCmd(names []string) host.Cmd {
	return host.Cmd{Argv: append([]string{"apt-get", "remove", "-y", "-q", "--"}, names...), Env: aptEnv}
}

var unitProps = "Id,LoadState,ActiveState,UnitFileState"

func systemctlShowCmd(units []string) host.Cmd {
	return host.Cmd{Argv: append([]string{"systemctl", "show", "--property=" + unitProps, "--"}, units...)}
}

// parseSystemctlShow parses `systemctl show` output: one block of
// Key=Value lines per unit, separated by blank lines, in argument order.
func parseSystemctlShow(out []byte) map[string]state.Unit {
	units := map[string]state.Unit{}
	var id string
	var u state.Unit
	flush := func() {
		if id != "" {
			units[id] = u
		}
		id, u = "", state.Unit{}
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		k, v, _ := strings.Cut(line, "=")
		switch k {
		case "Id":
			id = v
		case "LoadState":
			u.LoadState = v
		case "ActiveState":
			u.ActiveState = v
		case "UnitFileState":
			u.UnitFileState = v
		}
	}
	flush()
	return units
}

// parseGPGFingerprints extracts fingerprints from `gpg --with-colons`.
func parseGPGFingerprints(out []byte) []string {
	var fprs []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		f := strings.Split(sc.Text(), ":")
		if len(f) > 9 && f[0] == "fpr" {
			fprs = append(fprs, f[9])
		}
	}
	return fprs
}

// gpgShowKeysCmd reads a key's fingerprints with a throwaway GnuPG home,
// so nothing is left in /root/.gnupg.
func gpgShowKeysCmd(key []byte) host.Cmd {
	script := `d=$(mktemp -d) && trap 'rm -rf "$d"' EXIT && gpg --homedir "$d" --batch --show-keys --with-colons --with-fingerprint`
	return host.Cmd{Argv: []string{"/bin/sh", "-c", script}, Stdin: key}
}

// quoteIdent quotes a MariaDB identifier.
func quoteIdent(s string) string { return "`" + strings.ReplaceAll(s, "`", "``") + "`" }

// quoteString quotes a MariaDB string literal.
func quoteString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`, "\x00", `\0`, "\n", `\n`, "\r", `\r`).Replace(s) + "'"
}

func account(u *manifest.DatabaseUser) string { return quoteString(u.User) + "@" + quoteString(u.Host) }

func createDatabaseSQL(d *manifest.Database) string {
	return "CREATE DATABASE IF NOT EXISTS " + quoteIdent(d.Name) + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;\n"
}

func userSQL(u *manifest.DatabaseUser) string {
	var b strings.Builder
	acct := account(u)
	pw := quoteString(u.Password)
	fmt.Fprintf(&b, "CREATE USER IF NOT EXISTS %s IDENTIFIED BY %s;\n", acct, pw)
	fmt.Fprintf(&b, "ALTER USER %s IDENTIFIED BY %s;\n", acct, pw)
	for _, db := range u.Databases {
		fmt.Fprintf(&b, "GRANT ALL PRIVILEGES ON %s.* TO %s;\n", quoteIdent(db), acct)
	}
	return b.String()
}

func dropUserSQL(u *manifest.DatabaseUser) string {
	return "DROP USER IF EXISTS " + account(u) + ";\n"
}

const (
	selectSchemasSQL = "SELECT SCHEMA_NAME FROM information_schema.SCHEMATA;\n"
	selectUsersSQL   = "SELECT CONCAT(User, '@', Host) FROM mysql.user;\n"
)

// mariadbCmd runs SQL (on stdin, so passwords never appear in argv) against
// server, or the local server as root over the unix socket.
func mariadbCmd(server *manifest.DBServer, sql string) host.Cmd {
	argv := []string{"mariadb", "--batch", "--skip-column-names"}
	var env []string
	if server != nil {
		port := server.Port
		if port == 0 {
			port = 3306
		}
		argv = append(argv, "--host="+server.Host, "--port="+strconv.Itoa(port), "--user="+server.User)
		if server.TLS {
			argv = append(argv, "--ssl", "--ssl-verify-server-cert")
		}
		env = []string{"MYSQL_PWD=" + server.Password}
	}
	return host.Cmd{Argv: argv, Stdin: []byte(sql), Env: env}
}

func parseLines(out []byte) []string {
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

// serverKey groups database resources by the server they live on.
func serverKey(s *manifest.DBServer) string {
	if s == nil {
		return ""
	}
	return s.Host
}

// dbResourceName rebuilds a database resource name from an observed name.
func dbResourceName(s *manifest.DBServer, name string) string {
	if s == nil {
		return name
	}
	return s.Host + "/" + name
}

// letsEncryptLive is where certbot keeps a lineage's current certificate.
func letsEncryptLive(name string) string { return "/etc/letsencrypt/live/" + name + "/fullchain.pem" }

func certbotCmd(name string, c *manifest.Cert) host.Cmd {
	argv := []string{"certbot", "certonly", "--non-interactive", "--agree-tos",
		"--webroot", "--webroot-path", "/srv/www/acme",
		"--cert-name", name, "--keep-until-expiring", "--expand"}
	if c.Email != "" {
		argv = append(argv, "--email", c.Email)
	} else {
		argv = append(argv, "--register-unsafely-without-email")
	}
	for _, d := range c.Domains {
		argv = append(argv, "-d", d)
	}
	return host.Cmd{Argv: argv}
}
