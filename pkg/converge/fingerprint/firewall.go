package fingerprint

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// firewall reduces `nft -j list ruleset` to what is reachable from input
// hooks: the policy, which ports are accepted (and for which address
// families), whether loopback and established traffic are accepted, and
// which ports are rate limited. ferm/iptables-nft rules and native nftables
// tables therefore compare on effect, not representation.
func firewall(body string, add func(string, string)) {
	var doc struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(body)), &doc) != nil {
		return
	}
	type chainID struct{ family, table, name string }
	type chain struct {
		hook, policy string
		rules        [][]map[string]any
	}
	chains := map[chainID]*chain{}
	get := func(id chainID) *chain {
		if chains[id] == nil {
			chains[id] = &chain{}
		}
		return chains[id]
	}
	for _, obj := range doc.Nftables {
		if raw, ok := obj["chain"]; ok {
			var c struct{ Family, Table, Name, Hook, Policy string }
			if json.Unmarshal(raw, &c) == nil {
				ch := get(chainID{c.Family, c.Table, c.Name})
				ch.hook, ch.policy = c.Hook, c.Policy
			}
		}
		if raw, ok := obj["rule"]; ok {
			var r struct {
				Family, Table, Chain string
				Expr                 []map[string]any
			}
			if json.Unmarshal(raw, &r) == nil {
				ch := get(chainID{r.Family, r.Table, r.Chain})
				ch.rules = append(ch.rules, r.Expr)
			}
		}
	}
	families := func(f string) []string {
		if f == "inet" {
			return []string{"ip", "ip6"}
		}
		return []string{f}
	}
	facts := map[string][]string{}
	note := func(key, family string) {
		facts[key] = append(facts[key], families(family)...)
	}
	var visit func(id chainID, seen map[chainID]bool)
	visit = func(id chainID, seen map[chainID]bool) {
		ch := chains[id]
		if ch == nil || seen[id] {
			return
		}
		seen[id] = true
		for _, rule := range ch.rules {
			var ports []string
			var proto string
			accept, limited := false, false
			for _, e := range rule {
				for k, v := range e {
					switch k {
					case "accept":
						accept = true
					case "limit", "meter":
						limited = true
					case "set":
						// add/update @meter { saddr limit rate … }: per-source limit.
						if m, ok := v.(map[string]any); ok && (m["op"] == "update" || m["op"] == "add") && strings.Contains(fmt.Sprint(m["stmt"]), "limit") {
							limited = true
						}
					case "jump", "goto":
						if m, ok := v.(map[string]any); ok {
							visit(chainID{id.family, id.table, fmt.Sprint(m["target"])}, seen)
						}
					case "match":
						m, _ := v.(map[string]any)
						left, _ := m["left"].(map[string]any)
						if pl, ok := left["payload"].(map[string]any); ok && pl["field"] == "dport" {
							proto = fmt.Sprint(pl["protocol"])
							ports = append(ports, portValues(m["right"])...)
						}
						if meta, ok := left["meta"].(map[string]any); ok && (meta["key"] == "iif" || meta["key"] == "iifname") && fmt.Sprint(m["right"]) == "lo" {
							if hasVerdict(rule, "accept") {
								note("input/accept loopback", id.family)
							}
						}
						meta, isMeta := left["meta"].(map[string]any)
						pl, isPayload := left["payload"].(map[string]any)
						if ((isMeta && meta["key"] == "l4proto") || (isPayload && (pl["field"] == "protocol" || pl["field"] == "nexthdr"))) && hasVerdict(rule, "accept") {
							for _, p := range portValues(m["right"]) {
								switch p {
								case "icmp":
									facts["input/accept icmp"] = append(facts["input/accept icmp"], "ip")
								case "ipv6-icmp", "icmpv6":
									facts["input/accept icmpv6"] = append(facts["input/accept icmpv6"], "ip6")
								}
							}
						}
						if ct, ok := left["ct"].(map[string]any); ok && ct["key"] == "state" && strings.Contains(fmt.Sprint(m["right"]), "established") {
							if hasVerdict(rule, "accept") {
								note("input/accept established", id.family)
							}
						}
					}
				}
			}
			for _, p := range ports {
				if accept {
					note("input/accept "+proto+" "+p, id.family)
				}
				if limited {
					note("input/ratelimit "+proto+" "+p, id.family)
				}
			}
		}
	}
	for id, ch := range chains {
		if ch.hook == "input" {
			note("input/policy "+ch.policy, id.family)
			visit(id, map[chainID]bool{})
		}
	}
	for k, fs := range facts {
		slices.Sort(fs)
		add(k, strings.Join(slices.Compact(fs), ","))
	}
}

func hasVerdict(rule []map[string]any, verdict string) bool {
	for _, e := range rule {
		if _, ok := e[verdict]; ok {
			return true
		}
	}
	return false
}

func portValues(v any) []string {
	switch t := v.(type) {
	case float64:
		return []string{fmt.Sprint(int(t))}
	case string:
		return []string{t}
	case map[string]any:
		if set, ok := t["set"].([]any); ok {
			var out []string
			for _, x := range set {
				out = append(out, portValues(x)...)
			}
			return out
		}
		if r, ok := t["range"].([]any); ok && len(r) == 2 {
			lo, hi := portValues(r[0]), portValues(r[1])
			if len(lo) == 1 && len(hi) == 1 {
				return []string{lo[0] + "-" + hi[0]}
			}
		}
	}
	return nil
}

// iptables reduces iptables-save output (one "# family" marker per dump)
// to the same facts as firewall.
func iptables(body string, add func(string, string)) {
	type rule = iptRule
	facts := map[string][]string{}
	note := func(key, family string) { facts[key] = append(facts[key], family) }
	family := "ip"
	policies := map[string]string{}
	chains := map[string][]rule{}
	flush := func() {
		var visit func(chain string, via rule, seen map[string]bool)
		visit = func(chain string, via rule, seen map[string]bool) {
			if seen[chain] {
				return
			}
			seen[chain] = true
			for _, r := range chains[chain] {
				if r.proto == "" {
					r.proto = via.proto
				}
				if r.dport == "" {
					r.dport = via.dport
				}
				if r.src == "" {
					r.src = via.src
				}
				switch r.target {
				case "ACCEPT":
					switch {
					case r.iface == "lo":
						note("input/accept loopback", family)
					case r.established:
						note("input/accept established", family)
					case strings.HasPrefix(r.proto, "icmp") || r.proto == "ipv6-icmp":
						p := r.proto
						if p == "ipv6-icmp" {
							p = "icmpv6"
						}
						note("input/accept "+p, family)
					case r.dport != "":
						for _, p := range strings.Split(r.dport, ",") {
							key := "input/accept " + r.proto + " " + strings.ReplaceAll(p, ":", "-")
							if r.src != "" {
								key += " from " + r.src
							}
							note(key, family)
						}
					}
				case "DROP", "REJECT", "LOG", "RETURN", "":
				default:
					if r.limited || hasLimit(chains[r.target]) {
						for _, p := range strings.Split(r.dport, ",") {
							if p != "" {
								note("input/ratelimit "+r.proto+" "+p, family)
							}
						}
					}
					visit(r.target, r, seen)
				}
			}
		}
		if pol, ok := policies["INPUT"]; ok {
			if pol == "DROP" || pol == "ACCEPT" {
				// Only report a filtering firewall; an empty ACCEPT table is
				// the same as none.
				if pol == "DROP" || len(chains["INPUT"]) > 0 {
					note("input/policy "+strings.ToLower(pol), family)
				}
			}
			visit("INPUT", rule{}, map[string]bool{})
		}
		policies, chains = map[string]string{}, map[string][]rule{}
	}
	table := ""
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "# family "):
			flush()
			family = strings.TrimPrefix(l, "# family ")
		case strings.HasPrefix(l, "*"):
			table = strings.TrimPrefix(l, "*")
		case table != "filter":
		case strings.HasPrefix(l, ":"):
			f := strings.Fields(l[1:])
			if len(f) >= 2 {
				policies[f[0]] = f[1]
			}
		case strings.HasPrefix(l, "-A "):
			f := strings.Fields(l)
			var r rule
			chain := f[1]
			for i := 2; i < len(f); i++ {
				next := func() string {
					if i+1 < len(f) {
						i++
						return f[i]
					}
					return ""
				}
				switch f[i] {
				case "-p":
					r.proto = next()
				case "--dport", "--dports":
					r.dport = next()
				case "-s":
					r.src = next()
				case "-i":
					r.iface = next()
				case "-j":
					r.target = next()
				case "--ctstate", "--state":
					r.established = strings.Contains(next(), "ESTABLISHED")
				case "limit", "recent", "hashlimit":
					r.limited = true
				}
			}
			chains[chain] = append(chains[chain], r)
		}
	}
	flush()
	for k, fs := range facts {
		slices.Sort(fs)
		add(k, strings.Join(slices.Compact(fs), ","))
	}
}

type iptRule struct {
	proto, dport, src, iface, target string
	established, limited             bool
}

func hasLimit(rules []iptRule) bool {
	for _, r := range rules {
		if r.limited {
			return true
		}
	}
	return false
}
