// Package dnsdiag implements an opt-in, short-lived authoritative DNS observer.
// It observes recursive DNS peers; it never determines client device settings.
package dnsdiag

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	sessionTTL = 2 * time.Minute
	maxSessions = 4096
	maxPeerSessions = 3
	maxResolvers = 8
)

type Config struct {
	Zone string
	Nameserver string
	Listen string
	SinkIPv4 net.IP
}

func FromEnv() (Config, error) {
	zone := strings.TrimSpace(os.Getenv("NETDIAG_DNS_ZONE"))
	if zone == "" {
		return Config{}, nil
	}
	nameserver := strings.TrimSpace(os.Getenv("NETDIAG_DNS_NS"))
	listen := strings.TrimSpace(os.Getenv("NETDIAG_DNS_LISTEN"))
	sink := net.ParseIP(strings.TrimSpace(os.Getenv("NETDIAG_DNS_SINK_IPV4"))).To4()
	if !validHostname(zone) || !validHostname(nameserver) || listen == "" || sink == nil || !sink.IsGlobalUnicast() || sink.IsPrivate() {
		return Config{}, errors.New("DNS observer requires a valid zone, NS hostname, listen address, and public IPv4 sink")
	}
	if _, _, err := net.SplitHostPort(listen); err != nil {
		return Config{}, fmt.Errorf("DNS listen address: %w", err)
	}
	return Config{Zone: strings.ToLower(dns.Fqdn(zone)), Nameserver: strings.ToLower(dns.Fqdn(nameserver)), Listen: listen, SinkIPv4: sink}, nil
}

func validHostname(raw string) bool {
	raw = strings.TrimSuffix(raw, ".")
	if len(raw) == 0 || len(raw) > 250 || strings.ContainsAny(raw, ":/\\ ") {
		return false
	}
	for _, label := range strings.Split(raw, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
				return false
			}
		}
	}
	return true
}

type Session struct {
	ID string
	Host string
	ExpiresAt time.Time
	Resolvers []string
}

type entry struct {
	id string
	label string
	owner string
	expires time.Time
	resolvers map[string]struct{}
}

type Observer struct {
	mu sync.Mutex
	cfg Config
	byID map[string]*entry
	byLabel map[string]*entry
}

func New(c Config) *Observer {
	return &Observer{cfg: c, byID: make(map[string]*entry), byLabel: make(map[string]*entry)}
}

func randomLabel(bytes int) (string, error) {
	raw := make([]byte, bytes)
	if _, err := rand.Read(raw); err != nil { return "", err }
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)), nil
}

func (o *Observer) purgeLocked(now time.Time) {
	for id, s := range o.byID {
		if !now.Before(s.expires) {
			delete(o.byID, id)
			delete(o.byLabel, s.label)
		}
	}
}

func (o *Observer) Zone() string { return strings.TrimSuffix(o.cfg.Zone, ".") }

func (o *Observer) StartSession(peer string) (Session, error) {
	now := time.Now()
	o.mu.Lock()
	defer o.mu.Unlock()
	o.purgeLocked(now)
	if len(o.byID) >= maxSessions {
		return Session{}, errors.New("session_capacity_reached")
	}
	active := 0
	for _, s := range o.byID { if s.owner == peer { active++ } }
	if active >= maxPeerSessions { return Session{}, errors.New("peer_session_limit") }
	id, err := randomLabel(24)
	if err != nil { return Session{}, err }
	label, err := randomLabel(16)
	if err != nil { return Session{}, err }
	s := &entry{id: id, label: label, owner: peer, expires: now.Add(sessionTTL), resolvers: make(map[string]struct{})}
	o.byID[id] = s
	o.byLabel[label] = s
	return Session{ID: id, Host: label + "." + o.cfg.Zone[:len(o.cfg.Zone)-1], ExpiresAt: s.expires}, nil
}

func (o *Observer) Observe(question string, remote net.Addr) {
	ip, _, err := net.SplitHostPort(remote.String())
	if err != nil { return }
	addr, err := netip.ParseAddr(ip)
	if err != nil { return }
	name := strings.ToLower(dns.Fqdn(question))
	if !strings.HasSuffix(name, "."+o.cfg.Zone) { return }
	label := strings.TrimSuffix(name, "."+o.cfg.Zone)
	if label == "" || strings.Contains(label, ".") { return }
	now := time.Now()
	o.mu.Lock()
	defer o.mu.Unlock()
	o.purgeLocked(now)
	s := o.byLabel[label]
	if s != nil && len(s.resolvers) < maxResolvers {
		s.resolvers[addr.Unmap().String()] = struct{}{}
	}
}

func (o *Observer) Lookup(id, peer string) (Session, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.purgeLocked(time.Now())
	s := o.byID[id]
	if s == nil || s.owner != peer { return Session{}, false }
	result := Session{ID: s.id, ExpiresAt: s.expires}
	for ip := range s.resolvers { result.Resolvers = append(result.Resolvers, ip) }
	// Consistent order makes snapshots and API tests deterministic.
	sortStrings(result.Resolvers)
	return result, true
}

func sortStrings(items []string) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j] < items[j-1]; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func (o *Observer) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Authoritative = true
	m.RecursionAvailable = false
	m.Compress = true
	if r.Opcode != dns.OpcodeQuery || len(r.Question) != 1 {
		m.Rcode = dns.RcodeFormatError
		_ = w.WriteMsg(m)
		return
	}
	q := r.Question[0]
	name := strings.ToLower(dns.Fqdn(q.Name))
	if q.Qclass != dns.ClassINET || (name != o.cfg.Zone && !dns.IsSubDomain(o.cfg.Zone, name)) {
		m.Rcode = dns.RcodeRefused
		_ = w.WriteMsg(m)
		return
	}
	hdr := dns.RR_Header{Name: q.Name, Rrtype: q.Qtype, Class: dns.ClassINET, Ttl: 0}
	if name == o.cfg.Zone {
		switch q.Qtype {
		case dns.TypeNS:
			hdr.Rrtype = dns.TypeNS
			m.Answer = []dns.RR{&dns.NS{Hdr: hdr, Ns: o.cfg.Nameserver}}
		case dns.TypeSOA:
			hdr.Rrtype = dns.TypeSOA
			m.Answer = []dns.RR{&dns.SOA{Hdr: hdr, Ns: o.cfg.Nameserver, Mbox: "hostmaster." + o.cfg.Zone, Serial: 1, Refresh: 3600, Retry: 600, Expire: 86400, Minttl: 0}}
		}
	} else {
		label := strings.TrimSuffix(name, "." + o.cfg.Zone)
		o.mu.Lock()
		o.purgeLocked(time.Now())
		_, known := o.byLabel[label]
		o.mu.Unlock()
		if !known {
			m.Rcode = dns.RcodeNameError
		} else {
			if q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA {
				o.Observe(q.Name, w.RemoteAddr())
			}
			if q.Qtype == dns.TypeA {
				hdr.Rrtype = dns.TypeA
				m.Answer = []dns.RR{&dns.A{Hdr: hdr, A: o.cfg.SinkIPv4}}
			}
		}
	}
	_ = w.WriteMsg(m)
}

// Start binds UDP and TCP before returning and stops both listeners on cleanup.
func Start(ctx context.Context, cfg Config) (*Observer, func(), error) {
	if cfg.Zone == "" { return nil, func(){}, nil }
	udp, err := net.ListenPacket("udp", cfg.Listen)
	if err != nil { return nil, nil, err }
	tcp, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		_ = udp.Close()
		return nil, nil, err
	}
	observer := New(cfg)
	udpServer := &dns.Server{PacketConn: udp, Handler: observer}
	tcpServer := &dns.Server{Listener: tcp, Handler: observer}
	var wg sync.WaitGroup
	wg.Add(2)
	serve := func(srv *dns.Server) {
		defer wg.Done()
		if err := srv.ActivateAndServe(); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("DNS observer listener: %v", err)
		}
	}
	go serve(udpServer)
	go serve(tcpServer)
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			_ = udpServer.Shutdown()
			_ = tcpServer.Shutdown()
			_ = udp.Close()
			_ = tcp.Close()
			wg.Wait()
		})
	}
	go func() { <-ctx.Done(); cleanup() }()
	log.Printf("DNS observer enabled for %s on %s (UDP/TCP)", cfg.Zone, cfg.Listen)
	return observer, cleanup, nil
}
