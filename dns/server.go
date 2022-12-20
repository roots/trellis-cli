// This file has been adapted from https://github.com/norouter/norouter/blob/v0.6.4/pkg/agent/dns/dns.go

package dns

import (
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"

	"github.com/miekg/dns"
)

// Truncate for avoiding "Parse error" from `busybox nslookup`
// https://github.com/lima-vm/lima/issues/380
const truncateSize = 512

var defaultFallbackIPs = []string{"8.8.8.8", "1.1.1.1"}

type Network string

const (
	TCP Network = "tcp"
	UDP Network = "udp"
)

type HandlerOptions struct {
	IPv6            bool
	StaticDomains   map[string]string
	UpstreamServers []string
	TruncateReply   bool
}

type ServerOptions struct {
	HandlerOptions
	Address string
	TCPPort int
	UDPPort int
}

type Handler struct {
	truncate     bool
	clientConfig *dns.ClientConfig
	clients      []*dns.Client
	ipv6         bool
	domainToIP   map[string]net.IP
}

type Server struct {
	udp *dns.Server
	tcp *dns.Server
}

func newStaticClientConfig(ips []string) (*dns.ClientConfig, error) {
	s := ``
	for _, ip := range ips {
		s += fmt.Sprintf("nameserver %s\n", ip)
	}
	r := strings.NewReader(s)
	return dns.ClientConfigFromReader(r)
}

func NewHandler(opts HandlerOptions) (dns.Handler, error) {
	var cc *dns.ClientConfig
	var err error
	if len(opts.UpstreamServers) == 0 {
		if runtime.GOOS != "windows" {
			cc, err = dns.ClientConfigFromFile("/etc/resolv.conf")
			if err != nil {
				cc, err = newStaticClientConfig(defaultFallbackIPs)
				if err != nil {
					return nil, err
				}
			}
		} else {
			// For windows, the only fallback addresses are defaultFallbackIPs
			// since there is no /etc/resolv.conf
			cc, err = newStaticClientConfig(defaultFallbackIPs)
			if err != nil {
				return nil, err
			}
		}
	} else {
		if cc, err = newStaticClientConfig(opts.UpstreamServers); err != nil {
			if cc, err = newStaticClientConfig(defaultFallbackIPs); err != nil {
				return nil, err
			}
		}
	}
	clients := []*dns.Client{
		{}, // UDP
		{Net: "tcp"},
	}
	h := &Handler{
		truncate:     opts.TruncateReply,
		clientConfig: cc,
		clients:      clients,
		ipv6:         opts.IPv6,
		domainToIP:   make(map[string]net.IP),
	}
	for domain, address := range opts.StaticDomains {
		if ip := net.ParseIP(address); ip != nil {
			h.domainToIP[dns.CanonicalName(domain)] = ip
		}
	}
	return h, nil
}

func (h *Handler) handleQuery(w dns.ResponseWriter, req *dns.Msg) {
	var (
		reply   dns.Msg
		handled bool
	)
	reply.SetReply(req)
	for _, q := range req.Question {
		hdr := dns.RR_Header{
			Name:   q.Name,
			Rrtype: q.Qtype,
			Class:  q.Qclass,
			Ttl:    5,
		}
		qtype := q.Qtype
		switch q.Qtype {
		case dns.TypeAAAA:
			if !h.ipv6 {
				// A "correct" answer would be to set `handled = true` and return a NODATA response.
				// Unfortunately some older resolvers use a slow random source to set the transaction id.
				// This creates a problem on M1 computers, which are too fast for that implementation:
				// Both the A and AAAA queries might end up with the same id. Returning NODATA for AAAA
				// is faster, so would arrive first, and be treated as the response to the A query.
				// To avoid this, we will treat an AAAA query as an A query when IPv6 has been disabled.
				// This way it is either a valid response for an A query, or the A records will be discarded
				// by a genuine AAAA query, resulting in the desired NODATA response.
				qtype = dns.TypeA
			}
			fallthrough
		case dns.TypeA:
			var addrs []net.IP
			for domain, ip := range h.domainToIP {
				if dns.CompareDomainName(q.Name, domain) >= 1 {
					addrs = []net.IP{ip}
				}
			}
			for _, ip := range addrs {
				var a dns.RR
				ipv6 := ip.To4() == nil
				if qtype == dns.TypeA && !ipv6 {
					hdr.Rrtype = dns.TypeA
					a = &dns.A{
						Hdr: hdr,
						A:   ip.To4(),
					}
				} else if qtype == dns.TypeAAAA && ipv6 {
					hdr.Rrtype = dns.TypeAAAA
					a = &dns.AAAA{
						Hdr:  hdr,
						AAAA: ip.To16(),
					}
				} else {
					continue
				}
				reply.Answer = append(reply.Answer, a)
				handled = true
			}
		case dns.TypeTXT:
			txt, err := net.LookupTXT(q.Name)
			if err != nil {
				continue
			}
			for _, s := range txt {
				a := &dns.TXT{
					Hdr: hdr,
				}
				// Per RFC7208 3.3, when a TXT answer has multiple strings, the answer must be treated as
				// a single concatenated string. net.LookupTXT is pre-concatenating such answers, which
				// means we need to break it back up for this resolver to return a valid response.
				a.Txt = chunkify(s, 255)
				reply.Answer = append(reply.Answer, a)
				handled = true
			}
		case dns.TypeNS:
			ns, err := net.LookupNS(q.Name)
			if err != nil {
				continue
			}
			for _, s := range ns {
				if s.Host != "" {
					a := &dns.NS{
						Hdr: hdr,
						Ns:  s.Host,
					}
					reply.Answer = append(reply.Answer, a)
					handled = true
				}
			}
		case dns.TypeMX:
			mx, err := net.LookupMX(q.Name)
			if err != nil {
				continue
			}
			for _, s := range mx {
				if s.Host != "" {
					a := &dns.MX{
						Hdr:        hdr,
						Mx:         s.Host,
						Preference: s.Pref,
					}
					reply.Answer = append(reply.Answer, a)
					handled = true
				}
			}
		case dns.TypeSRV:
			_, addrs, err := net.LookupSRV("", "", q.Name)
			if err != nil {
				continue
			}
			hdr.Rrtype = dns.TypeSRV
			for _, addr := range addrs {
				a := &dns.SRV{
					Hdr:      hdr,
					Target:   addr.Target,
					Port:     addr.Port,
					Priority: addr.Priority,
					Weight:   addr.Weight,
				}
				reply.Answer = append(reply.Answer, a)
				handled = true
			}
		}
	}
	if handled {
		if h.truncate {
			reply.Truncate(truncateSize)
		}
		w.WriteMsg(&reply)
		return
	}
	h.handleDefault(w, req)
}

func (h *Handler) handleDefault(w dns.ResponseWriter, req *dns.Msg) {
	for _, client := range h.clients {
		for _, srv := range h.clientConfig.Servers {
			addr := fmt.Sprintf("%s:%s", srv, h.clientConfig.Port)
			reply, _, err := client.Exchange(req, addr)
			if err != nil {
				continue
			}
			if h.truncate {
				reply.Truncate(truncateSize)
			}
			w.WriteMsg(reply)
			return
		}
	}
	var reply dns.Msg
	reply.SetReply(req)
	if h.truncate {
		reply.Truncate(truncateSize)
	}
	w.WriteMsg(&reply)
}

func (h *Handler) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	switch req.Opcode {
	case dns.OpcodeQuery:
		h.handleQuery(w, req)
	default:
		h.handleDefault(w, req)
	}
}

func Start(opts ServerOptions) (*Server, error) {
	server := &Server{}
	if opts.UDPPort > 0 {
		udpSrv, err := listenAndServe(UDP, opts)
		if err != nil {
			return nil, err
		}
		server.udp = udpSrv
	}
	if opts.TCPPort > 0 {
		tcpSrv, err := listenAndServe(TCP, opts)
		if err != nil {
			return nil, err
		}
		server.tcp = tcpSrv
	}
	return server, nil
}

func listenAndServe(network Network, opts ServerOptions) (*dns.Server, error) {
	var addr string
	// always enable reply truncate for UDP
	if network == UDP {
		opts.HandlerOptions.TruncateReply = true
		addr = fmt.Sprintf("%s:%d", opts.Address, opts.UDPPort)
	} else {
		addr = fmt.Sprintf("%s:%d", opts.Address, opts.TCPPort)
	}
	h, err := NewHandler(opts.HandlerOptions)
	if err != nil {
		return nil, err
	}
	s := &dns.Server{Net: string(network), Addr: addr, Handler: h}
	go func() {
		log.Printf("DNS %v server listening on: %v", network, addr)
		if e := s.ListenAndServe(); e != nil {
			panic(e)
		}
	}()

	return s, nil
}

func chunkify(buffer string, limit int) []string {
	var result []string
	for len(buffer) > 0 {
		if len(buffer) < limit {
			limit = len(buffer)
		}
		result = append(result, buffer[:limit])
		buffer = buffer[limit:]
	}
	return result
}
