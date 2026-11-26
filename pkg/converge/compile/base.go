package compile

import (
	"fmt"
	"io/fs"
	"net"
	"strconv"
	"strings"

	"github.com/roots/trellis-cli/pkg/converge/config"
	"github.com/roots/trellis-cli/pkg/converge/lock"
	m "github.com/roots/trellis-cli/pkg/converge/manifest"
)

// Notifications shared across the compiler.
var (
	aptUpdate     = m.RunCommand("apt-get", "update")
	daemonReload  = m.RunCommand("systemctl", "daemon-reload")
	sysusersApply = m.RunCommand("systemd-sysusers")
	tmpfilesApply = m.RunCommand("systemd-tmpfiles", "--create", tmpfilesPath)
	nginxReload   = m.ReloadUnit("nginx.service", "nginx", "-t")
	sshReload     = m.ReloadUnit("ssh.service", "sshd", "-t")
	nftReload     = m.ReloadUnit("nftables.service", "nft", "-c", "-f", "/etc/nftables.conf")
	// Restart, not reload: fail2ban 1.0's reload drops a jail's actions when
	// its ban action changes, leaving the jail unable to ban. Bans persist
	// across restarts in fail2ban's database.
	fail2banReload  = m.RestartUnit("fail2ban.service", "fail2ban-client", "-t")
	journaldRestart = m.RestartUnit("systemd-journald.service")
	mariadbRestart  = m.RestartUnit("mariadb.service")

	enabled = m.Ptr(true)
)

const (
	tmpfilesPath = "/etc/tmpfiles.d/trellis.conf"
	sysusersPath = "/etc/sysusers.d/trellis.conf"
	keyringDir   = "/etc/apt/keyrings"
)

type aptRepo struct {
	name         string
	uri          string
	suite        string
	components   string
	keyURL       string
	keyFile      string
	fingerprints []string // every key in the file (see manifest.Download)
}

func (b *builder) repo(r aptRepo) {
	keyring := keyringDir + "/" + r.keyFile
	key := m.NewDownload(keyring, r.keyURL, 0o644).InPhase(m.PhaseRepos)
	key.Download.Fingerprints = r.fingerprints
	key.Notify = []m.Notify{aptUpdate}
	sources := b.render("apt/source.sources", map[string]string{
		"URI": r.uri, "Suite": r.suite, "Components": r.components, "Keyring": keyring,
	})
	b.add(key, m.NewRootFile("/etc/apt/sources.list.d/trellis-"+r.name+".sources", sources, 0o644, aptUpdate).InPhase(m.PhaseRepos))
}

func (b *builder) base() {
	if snap := b.in.Lock.AptSnapshot; snap != "" {
		b.add(m.NewRootFile("/etc/apt/apt.conf.d/50trellis-snapshot",
			"// Managed by Trellis: pin the Ubuntu archive to snapshot.ubuntu.com.\nAPT::Snapshot \""+snap+"\";\n",
			0o644, aptUpdate).InPhase(m.PhaseRepos))
	}

	b.pkg("ca-certificates", "curl", "git", "gnupg", "openssl", "unzip", "nftables", "fail2ban", "python3-systemd")
	b.pkg(b.env.Server.Packages...)

	b.users()
	b.ssh()
	b.firewall()
	b.fail2ban()
	b.mail()
	b.swap()

	// Security updates shouldn't wait for someone to provision.
	b.pkg("unattended-upgrades")
	b.add(m.NewRootFile("/etc/apt/apt.conf.d/20auto-upgrades",
		"// Managed by Trellis.\nAPT::Periodic::Update-Package-Lists \"1\";\nAPT::Periodic::Unattended-Upgrade \"1\";\n", 0o644))

	b.add(
		m.NewRootFile("/etc/systemd/journald.conf.d/50-trellis.conf", b.render("journald/journald.conf", nil), 0o644, journaldRestart),
		// First, before packages: services read the timezone when they
		// start, and apt starts them on install (MariaDB's system_time_zone).
		m.NewExec("timezone", []string{"timedatectl", "set-timezone", b.env.Server.Timezone}, b.env.Server.Timezone).InPhase(m.PhaseRepos),
		m.NewService("nftables.service", enabled, true),
		m.NewService("fail2ban.service", enabled, true),
	)
}

type siteDirs struct {
	Name   string
	Shared []string
}

func (b *builder) users() {
	var sites []siteDirs
	for _, s := range b.env.Sites {
		sd := siteDirs{Name: s.Name}
		for _, w := range s.Writable {
			sd.Shared = append(sd.Shared, sharedName(w))
		}
		sites = append(sites, sd)
	}
	type user struct{ Name, Group string }
	users := []user{{"admin", "admin"}, {"web", "www-data"}}
	data := map[string]any{"Sites": sites, "Users": users}
	b.add(
		m.NewRootFile(sysusersPath, b.render("users/sysusers.conf", data), 0o644, sysusersApply).InPhase(m.PhaseIdentity),
		m.NewRootFile(tmpfilesPath, b.render("users/tmpfiles.conf", data), 0o644, tmpfilesApply).InPhase(m.PhaseIdentity),
	)

	// Shell startup files from /etc/skel (sysusers doesn't copy them),
	// copied as the user so they own them; existing files are kept.
	for _, u := range users {
		skel := `for f in .bashrc .profile .bash_logout; do [ -e "/home/` + u.Name + `/$f" ] || cp "/etc/skel/$f" "/home/` + u.Name + `/$f"; done`
		b.add(m.NewExec("skel:"+u.Name, []string{"runuser", "-u", u.Name, "--", "/bin/sh", "-c", skel}, "1").InPhase(m.PhaseFiles))
	}

	admins := b.keys(b.env.Access.Admins, "access.admins")
	b.add(m.NewFile("/home/admin/.ssh/authorized_keys", admins, 0o600, "admin", "admin"))

	// admin's sudo rights come from the sudo group. Outside development it
	// needs its password (generated in secrets; the CLI supplies it to run
	// the agent), so a stolen SSH key alone doesn't give root.
	sudoers := "# Managed by Trellis.\n"
	if b.env.Name == config.DevelopmentEnv {
		sudoers += "admin ALL=(ALL) NOPASSWD:ALL\n"
	} else if pw := b.required("admin_password"); pw != "" {
		b.add(m.NewExec("admin-password", []string{"chpasswd"}, "").WithStdin("admin:" + pw + "\n").InPhase(m.PhaseFiles))
	}
	deploy := b.env.Access.Deploy
	if len(deploy) == 0 {
		deploy = b.env.Access.Admins
	}
	b.add(m.NewFile("/home/web/.ssh/authorized_keys", b.keys(deploy, "access.deploy"), 0o600, "web", "www-data"))
	// Exactly the reload deploys need; `service` for existing deploy hooks.
	v := b.env.Server.PHP.Version
	sudoers += "web ALL=(root) NOPASSWD: /usr/bin/systemctl reload php" + v + "-fpm.service, /usr/sbin/service php" + v + "-fpm reload\n"
	b.add(m.NewRootFile("/etc/sudoers.d/90-trellis", sudoers, 0o440).WithValidate("visudo", "-cf", "%s"))
}

// sharedName maps a writable release path to its directory under shared/.
func sharedName(p string) string { return config.SharedName(p) }

// keys resolves key references to authorized_keys content.
func (b *builder) keys(refs []string, where string) string {
	keys, errs := ResolveKeys(refs, b.in.Lock, b.in.Project)
	for _, err := range errs {
		b.errorf("%s: %v", where, err)
	}
	return "# Managed by Trellis.\n" + strings.Join(keys, "\n") + "\n"
}

// ResolveKeys expands key references into public keys: github:<user> (from
// trellis.lock), file:<path> (relative to the project), or a literal key
// (ssh-…, ecdsa-…, sk-…, age1…). Duplicates are dropped; order is kept.
func ResolveKeys(refs []string, l *lock.Lock, project fs.FS) ([]string, []error) {
	var keys []string
	var errs []error
	seen := map[string]bool{}
	add := func(k string) {
		k = strings.TrimSpace(k)
		if k != "" && !strings.HasPrefix(k, "#") && !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, ref := range refs {
		switch {
		case strings.HasPrefix(ref, "github:"):
			ks, ok := l.SSHKeys[ref]
			if !ok {
				errs = append(errs, fmt.Errorf("%s is not in trellis.lock (run `trellis converge lock`)", ref))
			}
			for _, k := range ks {
				add(k)
			}
		case strings.HasPrefix(ref, "file:"):
			if project == nil {
				errs = append(errs, fmt.Errorf("%s: no project directory", ref))
				continue
			}
			data, err := fs.ReadFile(project, strings.TrimPrefix(ref, "file:"))
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", ref, err))
				continue
			}
			for _, k := range strings.Split(string(data), "\n") {
				add(k)
			}
		case strings.HasPrefix(ref, "ssh-"), strings.HasPrefix(ref, "ecdsa-"), strings.HasPrefix(ref, "sk-"), strings.HasPrefix(ref, "age1"):
			add(ref)
		default:
			errs = append(errs, fmt.Errorf("%q must be github:<user>, file:<path> or a public key", ref))
		}
	}
	return keys, errs
}

func (b *builder) ssh() {
	conf := b.render("ssh/sshd.conf", map[string]any{"PermitRoot": false})
	// 10- sorts before cloud-init's 50-cloud-init.conf; sshd uses the first
	// value it sees for each keyword.
	b.add(m.NewRootFile("/etc/ssh/sshd_config.d/10-trellis.conf", conf, 0o644, sshReload))
}

func (b *builder) firewall() {
	ssh := b.env.Server.Security.SSH
	data := map[string]any{
		"SSHRateLimit": ssh.RateLimit != nil && *ssh.RateLimit,
		"SSHAny":       false,
		"HTTP3":        b.anySSL(),
	}
	var v4, v6 []string
	for _, a := range ssh.AllowFrom {
		if a == "any" {
			data["SSHAny"] = true
			continue
		}
		ip := net.ParseIP(a)
		if ip == nil {
			ip, _, _ = net.ParseCIDR(a)
		}
		if ip.To4() != nil {
			v4 = append(v4, a)
		} else {
			v6 = append(v6, a)
		}
	}
	data["SSHAllow4"], data["SSHAllow6"] = v4, v6

	b.add(m.NewRootFile("/etc/nftables.conf", b.render("nftables/nftables.conf", data), 0o755, nftReload))
}

func (b *builder) fail2ban() {
	f := b.env.Server.Security.Fail2ban
	b.add(m.NewRootFile("/etc/fail2ban/jail.d/trellis.local",
		b.render("fail2ban/jail.local", map[string]any{"WPLogin": f.WPLogin, "XMLRPC": f.XMLRPC}),
		0o644, fail2banReload))
	for _, name := range []string{"wordpress-wp-login", "wordpress-xmlrpc"} {
		b.add(m.NewRootFile("/etc/fail2ban/filter.d/"+name+".conf", b.render("fail2ban/"+name+".conf", nil), 0o644, fail2banReload))
	}
}

func (b *builder) mail() {
	mail := b.env.Server.Mail
	if mail == nil {
		return
	}
	host, port, err := net.SplitHostPort(mail.SMTP)
	if err != nil {
		host, port = mail.SMTP, "587"
	}
	if _, err := strconv.Atoi(port); err != nil {
		b.errorf("mail.smtp: invalid port in %q", mail.SMTP)
	}
	from := mail.From
	if from == "" {
		from = "admin@" + b.env.ServerName()
	}
	// msmtp-mta provides /usr/sbin/sendmail for cron, unattended-upgrades
	// and anything else that expects it.
	b.pkg("msmtp", "msmtp-mta")
	domain := from[strings.LastIndex(from, "@")+1:]
	conf := b.render("mail/msmtprc", map[string]string{
		"Host": host, "Port": port,
		"User":     b.secret(mail.User, "mail.user"),
		"Password": b.secret(mail.Password, "mail.password"),
		"From":     from,
		"Domain":   domain,
	})
	// PHP-FPM runs as web:www-data and needs to read it to send mail.
	b.add(m.NewFile("/etc/msmtprc", conf, 0o640, "root", "www-data").AsSensitive())
}

const swapUnit = "/etc/systemd/system/swapfile.swap"

// swap adds a swapfile. Small servers otherwise run out of memory during
// package installs, composer runs or MariaDB spikes. The size depends on
// the server's memory, so the script decides it there; it only creates
// /swapfile when missing and never resizes one.
func (b *builder) swap() {
	size := b.env.Server.Swap
	if size == "off" {
		return
	}
	create := `mem=$(awk '/^MemTotal:/ {print int($2/1024)}' /proc/meminfo); s=$(( mem < 2048 ? mem : 2048 )); [ "$s" -ge 512 ] || s=512; size="${s}M"`
	guard := `[ ! -e /swapfile ] && [ -z "$(swapon --noheadings --show=NAME)" ]`
	if size != "auto" {
		create = "size=" + size
		guard = "[ ! -e /swapfile ]"
	}
	script := "set -e\nif " + guard + "; then\n  " + create + "\n  fallocate -l \"$size\" /swapfile\n  chmod 600 /swapfile\n  mkswap /swapfile >/dev/null\nfi\nif [ -e /swapfile ]; then systemctl enable --now swapfile.swap; fi\n"
	b.add(
		m.NewRootFile(swapUnit, "# Managed by Trellis.\n[Unit]\nDescription=Trellis swap file\nConditionPathExists=/swapfile\n\n[Swap]\nWhat=/swapfile\n\n[Install]\nWantedBy=swap.target\n", 0o644, daemonReload),
		m.NewExec("swapfile", []string{"/bin/sh", "-c", script}, size),
	)
}

// anySSL reports whether any site on this node serves HTTPS.
func (b *builder) anySSL() bool {
	for _, s := range b.env.Sites {
		if s.SSL != config.SSLNone {
			return true
		}
	}
	return false
}
