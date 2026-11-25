package fingerprint

import (
	"strings"
	"testing"
)

func TestNginxResolvesIncludesAndKeysByServer(t *testing.T) {
	// Two layouts of the same effective config: one file vs. includes.
	single := `=== nginx
# configuration file /etc/nginx/nginx.conf:
http {
  server {
    listen 80;
    server_name example.com;
    add_header X-Frame-Options SAMEORIGIN;
    location ~ \.php$ { fastcgi_read_timeout 600s; }
  }
}
`
	split := `=== nginx
# configuration file /etc/nginx/nginx.conf:
http {
  include sites-enabled/*.conf; # comment
}

# configuration file /etc/nginx/sites-enabled/example.com.conf:
server {
  server_name example.com;
  listen 80;
  include h5bp/headers.conf;
  location ~ \.php$ {
    fastcgi_read_timeout 600s;
  }
}

# configuration file /etc/nginx/h5bp/headers.conf:
add_header X-Frame-Options "SAMEORIGIN";
`
	a, b := Parse(single), Parse(split)
	if len(a) == 0 || len(a) != len(b) {
		t.Fatalf("facts differ:\n%v\n%v", a, b)
	}
	for k, v := range a {
		if b[k] != v {
			t.Errorf("%s: %q vs %q", k, v, b[k])
		}
	}
	want := "nginx/http > server[80 | example.com] > location[~ \\.php$] > fastcgi_read_timeout"
	if a[want] != "600s" {
		t.Errorf("missing %s in %v", want, a)
	}
	if a["nginx/http > server[80 | example.com] > add_header X-Frame-Options"] != "SAMEORIGIN" {
		t.Errorf("header not keyed: %v", a)
	}
}

func TestFirewallComparesEffectNotRepresentation(t *testing.T) {
	native := `=== firewall
{"nftables":[{"chain":{"family":"inet","table":"trellis","name":"input","hook":"input","policy":"drop"}},
{"rule":{"family":"inet","table":"trellis","chain":"input","expr":[{"match":{"op":"in","left":{"ct":{"key":"state"}},"right":["established","related"]}},{"accept":null}]}},
{"rule":{"family":"inet","table":"trellis","chain":"input","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":{"set":[80,443]}}},{"accept":null}]}},
{"rule":{"family":"inet","table":"trellis","chain":"input","expr":[{"match":{"op":"==","left":{"meta":{"key":"l4proto"}},"right":{"set":["icmp","ipv6-icmp"]}}},{"accept":null}]}},
{"rule":{"family":"inet","table":"trellis","chain":"input","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":22}},{"set":{"op":"add","elem":{"payload":{"protocol":"ip","field":"saddr"}},"set":"@m","stmt":[{"limit":{"rate":20,"per":"minute","inv":true}}]}},{"drop":null}]}}]}
`
	legacy := `=== firewall
{"nftables":[
{"chain":{"family":"ip","table":"filter","name":"INPUT","hook":"input","policy":"drop"}},
{"chain":{"family":"ip6","table":"filter","name":"INPUT","hook":"input","policy":"drop"}},
{"chain":{"family":"ip","table":"filter","name":"web"}},
{"rule":{"family":"ip","table":"filter","chain":"INPUT","expr":[{"jump":{"target":"web"}}]}},
{"rule":{"family":"ip","table":"filter","chain":"web","expr":[{"match":{"op":"==","left":{"payload":{"protocol":"tcp","field":"dport"}},"right":80}},{"accept":null}]}}]}
`
	n, l := Parse(native), Parse(legacy)
	if n["firewall/input/accept tcp 80"] != "ip,ip6" || n["firewall/input/policy drop"] != "ip,ip6" {
		t.Errorf("native: %v", n)
	}
	if l["firewall/input/accept tcp 80"] != "ip" {
		t.Errorf("jumped-to chain not followed: %v", l)
	}
	if n["firewall/input/ratelimit tcp 22"] != "ip,ip6" || n["firewall/input/accept icmp"] != "ip" || n["firewall/input/accept icmpv6"] != "ip6" {
		t.Errorf("rate limit / icmp: %v", n)
	}
	if n["firewall/input/accept established"] != "ip,ip6" {
		t.Errorf("established: %v", n)
	}
}

func TestHostnameIsNormalized(t *testing.T) {
	f := Parse("=== hostname\nweb-1\n=== hosts\n127.0.1.1 web-1\n=== sysctl\nkernel.hostname = web-1\n")
	if _, ok := f["hosts/127.0.1.1 $HOSTNAME"]; !ok || f["sysctl/kernel.hostname"] != "$HOSTNAME" {
		t.Errorf("%v", f)
	}
}

func TestCompareClassifiesAgainstBaselines(t *testing.T) {
	base := Facts{"pkg/vim": "1", "sshd/permitrootlogin": "yes", "sysctl/fs.file-nr": "1"}
	trellis := Facts{"pkg/vim": "1", "sshd/permitrootlogin": "yes", "pkg/nginx": "1.26", "pkg/chrony": "4", "swap/x": "1G", "sysctl/fs.file-nr": "9"}
	converge := Facts{"pkg/vim": "1", "sshd/permitrootlogin": "no", "pkg/nginx": "1.26", "sysctl/fs.file-nr": "7"}
	cfg, err := LoadConfig([]byte("ignore: ['^sysctl/fs\\.file-nr$']\nexpected:\n  - key: '^pkg/chrony$'\n    reason: systemd-timesyncd instead\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Change{}
	for _, c := range Compare(base, trellis, base, converge, cfg) {
		got[c.Key] = c
	}
	check := func(k string, kind Kind) {
		t.Helper()
		if got[k].Kind != kind {
			t.Errorf("%s = %q, want %q", k, got[k].Kind, kind)
		}
	}
	check("pkg/nginx", Same)
	check("sshd/permitrootlogin", OnlyConverge)
	check("swap/x", OnlyTrellis)
	check("pkg/chrony", OnlyTrellis)
	if got["pkg/chrony"].Accepted == "" {
		t.Error("expectation not applied")
	}
	if _, ok := got["pkg/vim"]; ok {
		t.Error("untouched fact reported")
	}
	if _, ok := got["sysctl/fs.file-nr"]; ok {
		t.Error("ignored fact reported")
	}
	var b strings.Builder
	Report(&b, Compare(base, trellis, base, converge, cfg))
	if !strings.Contains(b.String(), "| sshd | 0 | 0 | 0 | 1 | 0 |") {
		t.Errorf("report:\n%s", b.String())
	}
}

func TestLegacyIptablesMatchesNftFacts(t *testing.T) {
	legacy := `=== firewall-legacy
# family ip
*filter
:INPUT DROP [0:0]
:dport-limit-ssh - [0:0]
-A INPUT -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
-A INPUT -i lo -j ACCEPT
-A INPUT -p icmp -j ACCEPT
-A INPUT -s 10.0.0.1/32 -p tcp -m tcp --dport 22 -j ACCEPT
-A INPUT -p tcp -m tcp --dport 22 -j dport-limit-ssh
-A INPUT -p tcp -m tcp --dport 22 -j ACCEPT
-A INPUT -p tcp -m multiport --dports 80,443 -j ACCEPT
-A dport-limit-ssh -m recent --update --seconds 60 -j DROP
COMMIT
# family ip6
`
	f := Parse(legacy)
	for k, want := range map[string]string{
		"firewall/input/policy drop":                    "ip",
		"firewall/input/accept established":             "ip",
		"firewall/input/accept loopback":                "ip",
		"firewall/input/accept icmp":                    "ip",
		"firewall/input/accept tcp 22":                  "ip",
		"firewall/input/accept tcp 443":                 "ip",
		"firewall/input/accept tcp 22 from 10.0.0.1/32": "ip",
		"firewall/input/ratelimit tcp 22":               "ip",
	} {
		if f[k] != want {
			t.Errorf("%s = %q, want %q (all: %v)", k, f[k], want, f)
		}
	}
}
