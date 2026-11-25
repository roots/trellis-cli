// Package fingerprint compares servers by their effective configuration
// rather than their files. probe.sh asks each tool for its resolved view
// (nginx -T, sshd -T, php-fpm -tt, SHOW GLOBAL VARIABLES, sysctl, dpkg,
// systemctl, nft, sudo -l, …); Parse normalizes that into sorted
// "area/key" → value facts that don't depend on how the config was split
// into files. Compare then diffs two provisioners against their own
// pre-provisioning baselines, so only what each provisioner changed counts.
package fingerprint

import (
	_ "embed"
	"regexp"
	"slices"
	"strings"
)

//go:embed probe.sh
var Probe string

// Facts are normalized key → value pairs, keyed "area/name".
type Facts map[string]string

// Sections splits probe output on "=== name" lines.
func Sections(raw string) map[string]string {
	out := map[string]string{}
	var name string
	var body strings.Builder
	flush := func() {
		if name != "" {
			out[name] = body.String()
		}
		body.Reset()
	}
	for _, line := range strings.Split(raw, "\n") {
		if n, ok := strings.CutPrefix(line, "=== "); ok {
			flush()
			name = strings.TrimSpace(n)
			continue
		}
		body.WriteString(line)
		body.WriteByte('\n')
	}
	flush()
	return out
}

type normalizer func(body string, add func(key, value string))

var normalizers = map[string]normalizer{
	"os":              lines(func(l string, add adder) { add("release", l) }),
	"packages":        tsv(2, func(f []string, add adder) { add(f[0], f[1]) }),
	"apt-sources":     lines(func(l string, add adder) { add(l, "present") }),
	"units-enabled":   tsv(2, func(f []string, add adder) { add(f[0], f[1]) }),
	"units-active":    lines(func(l string, add adder) { add(l, "active") }),
	"listen":          tsv(3, listen),
	"users":           tsv(2, func(f []string, add adder) { add(f[0], f[1]) }),
	"sudo":            grouped(),
	"authorized-keys": grouped(),
	"sshd":            sshd,
	"nginx":           nginx,
	"php-fpm-pools":   phpFPM,
	"php-ini-fpm":     tsv(2, func(f []string, add adder) { add(f[0], f[1]) }),
	"php-ini-cli":     tsv(2, func(f []string, add adder) { add(f[0], f[1]) }),
	"php-modules-fpm": lines(func(l string, add adder) {
		if !strings.HasPrefix(l, "[") {
			add(l, "loaded")
		}
	}),
	"mariadb-vars":    tsv(2, func(f []string, add adder) { add(f[0], f[1]) }),
	"mariadb-dbs":     lines(func(l string, add adder) { add(l, "present") }),
	"mariadb-grants":  grants,
	"fail2ban":        tsv(3, func(f []string, add adder) { add(f[0]+"/"+f[1], f[2]) }),
	"firewall":        firewall,
	"firewall-legacy": iptables,
	"sysctl":          keyValue(" = "),
	"timedate":        keyValue("="),
	"journald":        journald,
	"swap":            fields(2, func(f []string, add adder) { add(f[0], f[1]) }),
	"timers":          lines(func(l string, add adder) { add(l, "present") }),
	"cron":            tsv(2, func(f []string, add adder) { add(f[0]+"/"+f[1], "present") }),
	"tree":            tsv(2, func(f []string, add adder) { add(f[0], f[1]) }),
	"logrotate":       logrotate,
	"hosts":           lines(func(l string, add adder) { add(strings.Join(strings.Fields(l), " "), "present") }),
	"msmtp":           msmtp,
	"tools":           keyValue("="),
	"http":            http,
	"reboot-required": lines(func(l string, add adder) {
		add("pending", l)
	}),
}

type adder = func(key, value string)

// areaAlias reports some sections under another area so equivalent
// facts from different tools compare (legacy iptables vs nftables).
var areaAlias = map[string]string{"firewall-legacy": "firewall"}

// Parse normalizes probe output into facts. The server's hostname is
// replaced by $HOSTNAME so servers with different names compare equal.
func Parse(raw string) Facts {
	secs := Sections(raw)
	host := strings.TrimSpace(secs["hostname"])
	if host == "" {
		// Older probes: fall back to sysctl.
		for _, l := range strings.Split(secs["sysctl"], "\n") {
			if v, ok := strings.CutPrefix(l, "kernel.hostname = "); ok {
				host = strings.TrimSpace(v)
			}
		}
	}
	f := Facts{}
	for name, body := range secs {
		norm := normalizers[name]
		if norm == nil {
			continue
		}
		norm(body, func(key, value string) {
			if host != "" {
				key = strings.ReplaceAll(key, host, "$HOSTNAME")
				value = strings.ReplaceAll(value, host, "$HOSTNAME")
			}
			area := name
			if a, ok := areaAlias[name]; ok {
				area = a
			}
			k := area + "/" + key
			if old, ok := f[k]; ok && old != value {
				value = old + " | " + value
			}
			f[k] = value
		})
	}
	return f
}

// Keys returns the fact keys in order.
func (f Facts) Keys() []string {
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func lines(fn func(string, adder)) normalizer {
	return func(body string, add func(string, string)) {
		for _, l := range strings.Split(body, "\n") {
			if l = strings.TrimSpace(l); l != "" {
				fn(l, add)
			}
		}
	}
}

func tsv(n int, fn func([]string, adder)) normalizer {
	return lines(func(l string, add adder) {
		f := strings.SplitN(l, "\t", n)
		for len(f) < n {
			f = append(f, "")
		}
		fn(f, add)
	})
}

func fields(n int, fn func([]string, adder)) normalizer {
	return lines(func(l string, add adder) {
		f := strings.Fields(l)
		if len(f) >= n {
			fn(f, add)
		}
	})
}

func keyValue(sep string) normalizer {
	return lines(func(l string, add adder) {
		if k, v, ok := strings.Cut(l, sep); ok {
			add(strings.TrimSpace(k), strings.TrimSpace(v))
		}
	})
}

// grouped collects "key\tvalue" lines into one sorted value per key.
func grouped() normalizer {
	return func(body string, add func(string, string)) {
		m := map[string][]string{}
		for _, l := range strings.Split(body, "\n") {
			if k, v, ok := strings.Cut(strings.TrimSpace(l), "\t"); ok {
				m[k] = append(m[k], strings.TrimSpace(v))
			}
		}
		for k, vs := range m {
			slices.Sort(vs)
			add(k, strings.Join(slices.Compact(vs), "; "))
		}
	}
}

var procRe = regexp.MustCompile(`\("([^"]+)"`)

func listen(f []string, add adder) {
	var procs []string
	for _, m := range procRe.FindAllStringSubmatch(f[2], -1) {
		procs = append(procs, m[1])
	}
	slices.Sort(procs)
	add(f[0]+" "+f[1], strings.Join(slices.Compact(procs), ","))
}

func sshd(body string, add func(string, string)) {
	m := map[string][]string{}
	for _, l := range strings.Split(body, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), " "); ok {
			m[k] = append(m[k], v)
		}
	}
	for k, vs := range m {
		slices.Sort(vs)
		add(k, strings.Join(vs, ", "))
	}
}

var fpmPrefix = regexp.MustCompile(`^\[[^\]]*\] NOTICE:\s*`)

func phpFPM(body string, add func(string, string)) {
	pool := ""
	for _, l := range strings.Split(body, "\n") {
		l = fpmPrefix.ReplaceAllString(l, "")
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			pool = strings.Trim(t, "[]")
			continue
		}
		if k, v, ok := strings.Cut(t, " = "); ok && pool != "" {
			add(pool+"/"+strings.TrimSpace(k), strings.TrimSpace(v))
		}
	}
}

var passwordRe = regexp.MustCompile(`PASSWORD '[^']*'`)

func grants(body string, add func(string, string)) {
	m := map[string][]string{}
	for _, l := range strings.Split(body, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "\t"); ok {
			m[k] = append(m[k], passwordRe.ReplaceAllString(v, "PASSWORD '…'"))
		}
	}
	for k, vs := range m {
		slices.Sort(vs)
		add(k, strings.Join(vs, "; "))
	}
}

// journald takes the effective value of each setting: the last
// uncommented assignment across journald.conf and its drop-ins.
func journald(body string, add func(string, string)) {
	m := map[string]string{}
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "[") {
			continue
		}
		if k, v, ok := strings.Cut(l, "="); ok {
			m[k] = v
		}
	}
	for k, v := range m {
		add(k, v)
	}
}

func logrotate(body string, add func(string, string)) {
	for _, l := range strings.Split(body, "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(l), "rotating pattern: ")
		if !ok {
			continue
		}
		// "PATTERN  after 1 days empty log files are rotated, (7 rotations)..."
		pattern, policy, _ := strings.Cut(rest, "  ")
		add(pattern, strings.Join(strings.Fields(policy), " "))
	}
}

func msmtp(body string, add func(string, string)) {
	for _, l := range strings.Split(body, "\n") {
		f := strings.Fields(l)
		if len(f) > 0 {
			add(f[0], strings.Join(f[1:], " "))
		}
	}
}

func http(body string, add func(string, string)) {
	for _, l := range strings.Split(body, "\n") {
		url, rest, ok := strings.Cut(strings.TrimSpace(l), "\t")
		if !ok {
			continue
		}
		if s, ok := strings.CutPrefix(rest, "status="); ok {
			add(url+" status", s)
		} else if h, ok := strings.CutPrefix(rest, "header "); ok {
			name, value, _ := strings.Cut(h, ":")
			add(url+" "+strings.ToLower(strings.TrimSpace(name)), strings.TrimSpace(value))
		}
	}
}
