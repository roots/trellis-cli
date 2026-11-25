package fingerprint

import (
	"path"
	"slices"
	"strconv"
	"strings"
)

// nginx turns `nginx -T` output (every config file, not inlined) into one
// effective tree, with includes resolved, then into facts keyed by context:
//
//	http > server[443 ssl example.com] > location[~ \.php$] > fastcgi_read_timeout
//
// Servers are identified by their listen addresses and names, not by
// which file they came from, so differently organized configs compare.
func nginx(body string, add func(string, string)) {
	files := map[string]string{}
	var order []string
	var cur string
	var b strings.Builder
	flush := func() {
		if cur != "" {
			files[cur] = b.String()
			order = append(order, cur)
		}
		b.Reset()
	}
	for _, l := range strings.Split(body, "\n") {
		if p, ok := strings.CutPrefix(l, "# configuration file "); ok && strings.HasSuffix(p, ":") {
			flush()
			cur = strings.TrimSuffix(p, ":")
			continue
		}
		b.WriteString(l)
		b.WriteByte('\n')
	}
	flush()
	if len(order) == 0 {
		return
	}
	parsed := map[string][]directive{}
	for p, src := range files {
		parsed[p] = parseNginx(src)
	}
	root := expandIncludes(parsed[order[0]], parsed, 0)
	walkNginx(root, "", add)
}

type directive struct {
	name  string
	args  []string
	block []directive // nil for simple directives
}

func tokenizeNginx(src string) []string {
	var toks []string
	i := 0
	for i < len(src) {
		c := src[i]
		switch c {
		case '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case ' ', '\t', '\n', '\r':
			i++
		case '{', '}', ';':
			toks = append(toks, string(c))
			i++
		case '"', '\'':
			j := i + 1
			for j < len(src) && src[j] != c {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			toks = append(toks, src[i+1:min(j, len(src))])
			i = j + 1
		default:
			j := i
			for j < len(src) && !strings.ContainsRune(" \t\n\r{};", rune(src[j])) {
				if src[j] == '\\' {
					j++
				}
				j++
			}
			toks = append(toks, src[i:min(j, len(src))])
			i = j
		}
	}
	return toks
}

func parseNginx(src string) []directive {
	toks := tokenizeNginx(src)
	pos := 0
	var parse func() []directive
	parse = func() []directive {
		var out []directive
		var words []string
		for pos < len(toks) {
			t := toks[pos]
			pos++
			switch t {
			case ";":
				if len(words) > 0 {
					out = append(out, directive{name: words[0], args: words[1:]})
				}
				words = nil
			case "{":
				d := directive{block: []directive{}}
				if len(words) > 0 {
					d.name, d.args = words[0], words[1:]
				}
				d.block = parse()
				out = append(out, d)
				words = nil
			case "}":
				return out
			default:
				words = append(words, t)
			}
		}
		return out
	}
	return parse()
}

func expandIncludes(ds []directive, files map[string][]directive, depth int) []directive {
	if depth > 16 {
		return ds
	}
	var out []directive
	for _, d := range ds {
		if d.name == "include" && d.block == nil && len(d.args) == 1 {
			pattern := d.args[0]
			if !path.IsAbs(pattern) {
				pattern = path.Join("/etc/nginx", pattern)
			}
			var matches []string
			for p := range files {
				if ok, _ := path.Match(pattern, p); ok {
					matches = append(matches, p)
				}
			}
			slices.Sort(matches)
			for _, m := range matches {
				out = append(out, expandIncludes(files[m], files, depth+1)...)
			}
			continue
		}
		if d.block != nil {
			d.block = expandIncludes(d.block, files, depth+1)
		}
		out = append(out, d)
	}
	return out
}

// keyed directives can repeat with different first arguments; each is its
// own fact (e.g. one per header).
var keyed = map[string]bool{
	"add_header": true, "fastcgi_param": true, "proxy_set_header": true, "set": true,
	"listen": true, "error_page": true, "fastcgi_cache_valid": true, "log_format": true,
	"limit_req_zone": true, "limit_conn_zone": true, "fastcgi_cache_path": true,
	"more_set_headers": true, "ssl_conf_command": true, "env": true, "load_module": true,
}

func blockKey(d directive) string {
	switch d.name {
	case "server":
		var listens, names []string
		for _, c := range d.block {
			switch c.name {
			case "listen":
				listens = append(listens, strings.Join(c.args, " "))
			case "server_name":
				names = append(names, c.args...)
			}
		}
		slices.Sort(listens)
		slices.Sort(names)
		return "server[" + strings.Join(listens, ", ") + " | " + strings.Join(names, " ") + "]"
	case "http", "events", "stream":
		return d.name
	}
	return d.name + "[" + strings.Join(d.args, " ") + "]"
}

func walkNginx(ds []directive, ctx string, add func(string, string)) {
	prefix := ctx
	if prefix != "" {
		prefix += " > "
	}
	counts := map[string]int{}
	for _, d := range ds {
		if d.block != nil {
			if d.name == "types" {
				continue // mime types
			}
			walkNginx(d.block, prefix+blockKey(d), add)
			continue
		}
		key, value := d.name, strings.Join(d.args, " ")
		switch {
		case keyed[d.name] && len(d.args) > 0:
			key, value = d.name+" "+d.args[0], strings.Join(d.args[1:], " ")
		case d.name == "server_name" || d.name == "index" || d.name == "gzip_types":
			args := slices.Clone(d.args)
			slices.Sort(args)
			value = strings.Join(args, " ")
		}
		counts[key]++
		if counts[key] > 1 && !keyed[d.name] {
			// Order-sensitive repeats (allow/deny, rewrite): keep their order.
			key = key + " #" + strconv.Itoa(counts[key])
		}
		add(prefix+key, value)
	}
}
