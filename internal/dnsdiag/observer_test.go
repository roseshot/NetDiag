package dnsdiag

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type responseWriter struct {
	answer *dns.Msg
	remote net.Addr
}

func (w *responseWriter) LocalAddr() net.Addr         { return &net.UDPAddr{} }
func (w *responseWriter) RemoteAddr() net.Addr        { return w.remote }
func (w *responseWriter) WriteMsg(m *dns.Msg) error   { w.answer = m; return nil }
func (w *responseWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *responseWriter) Close() error                { return nil }
func (w *responseWriter) TsigStatus() error           { return nil }
func (w *responseWriter) TsigTimersOnly(bool)         {}
func (w *responseWriter) Hijack()                     {}

func testConfig() Config {
	return Config{Zone: "observer.example.test.", Nameserver: "ns1.example.test.", Listen: "127.0.0.1:0", SinkIPv4: net.ParseIP("93.184.216.34")}
}

func TestOptionalConfigurationAndValidation(t *testing.T) {
	t.Setenv("NETDIAG_DNS_ZONE", "")
	if cfg, err := FromEnv(); err != nil || cfg.Zone != "" {
		t.Fatalf("unset observer must be disabled: %+v, %v", cfg, err)
	}
	t.Setenv("NETDIAG_DNS_ZONE", "observer.example.test")
	t.Setenv("NETDIAG_DNS_NS", "ns1.example.test")
	t.Setenv("NETDIAG_DNS_LISTEN", "127.0.0.1:1053")
	t.Setenv("NETDIAG_DNS_SINK_IPV4", "93.184.216.34")
	cfg, err := FromEnv()
	if err != nil || cfg.Zone != "observer.example.test." {
		t.Fatalf("valid configuration rejected: %+v, %v", cfg, err)
	}
	t.Setenv("NETDIAG_DNS_SINK_IPV4", "127.0.0.1")
	if _, err := FromEnv(); err == nil {
		t.Fatal("loopback sink must not be accepted")
	}
	for _, hostname := range []string{"", "-wrong.example", "bad..domain", "http://unsafe.example", "x_underscore.test"} {
		if validHostname(hostname) {
			t.Errorf("accepted invalid hostname %q", hostname)
		}
	}
}

func TestSessionIsolationAndExpiry(t *testing.T) {
	o := New(testConfig())
	s, err := o.StartSession("192.0.2.7")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.ID) != 39 || !strings.HasSuffix(s.Host, ".observer.example.test") {
		t.Fatalf("unexpected session: %+v", s)
	}
	if _, ok := o.Lookup(s.ID, "192.0.2.8"); ok {
		t.Fatal("other peer accessed session")
	}
	o.Observe(s.Host+".", &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 5353})
	o.Observe(s.Host+".", &net.UDPAddr{IP: net.ParseIP("8.8.8.8"), Port: 5354})
	data, ok := o.Lookup(s.ID, "192.0.2.7")
	if !ok || len(data.Resolvers) != 1 || data.Resolvers[0] != "8.8.8.8" {
		t.Fatalf("observation did not correlate uniquely: %+v", data)
	}
	for i := 0; i < maxPeerSessions-1; i++ {
		if _, err := o.StartSession("192.0.2.7"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := o.StartSession("192.0.2.7"); err == nil {
		t.Fatal("missing peer session limit")
	}
	o.mu.Lock()
	o.byID[s.ID].expires = time.Now().Add(-time.Second)
	o.mu.Unlock()
	if _, ok := o.Lookup(s.ID, "192.0.2.7"); ok {
		t.Fatal("expired session remained readable")
	}
}

func TestAuthoritativeResponsesAndNoOpenRecursion(t *testing.T) {
	o := New(testConfig())
	s, err := o.StartSession("192.0.2.7")
	if err != nil {
		t.Fatal(err)
	}
	call := func(name string, qtype uint16) *dns.Msg {
		t.Helper()
		q := new(dns.Msg)
		q.SetQuestion(dns.Fqdn(name), qtype)
		w := &responseWriter{remote: &net.UDPAddr{IP: net.ParseIP("9.9.9.9"), Port: 5353}}
		o.ServeDNS(w, q)
		if w.answer == nil {
			t.Fatal("no DNS reply")
		}
		return w.answer
	}
	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
		r := call(s.Host, qtype)
		if r.Rcode != dns.RcodeSuccess || !r.Authoritative || r.RecursionAvailable {
			t.Fatalf("invalid authoritative reply for %d: %+v", qtype, r)
		}
		if qtype == dns.TypeA && len(r.Answer) != 1 {
			t.Fatal("A query must receive the configured IPv4 sink")
		}
	}
	if r := call("other.example.test", dns.TypeA); r.Rcode != dns.RcodeRefused {
		t.Fatalf("foreign zone not refused: %d", r.Rcode)
	}
	if r := call("unknown.observer.example.test", dns.TypeA); r.Rcode != dns.RcodeNameError {
		t.Fatalf("unknown session should not be wildcard answerable: %d", r.Rcode)
	}
	if r := call("observer.example.test", dns.TypeNS); len(r.Answer) != 1 {
		t.Fatalf("missing authoritative zone NS: %+v", r)
	}
	data, _ := o.Lookup(s.ID, "192.0.2.7")
	if len(data.Resolvers) != 1 || data.Resolvers[0] != "9.9.9.9" {
		t.Fatalf("DNS query not associated with session: %+v", data)
	}
}

func TestDisabledListenerAndShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if observer, cleanup, err := Start(ctx, Config{}); err != nil || observer != nil {
		t.Fatalf("disabled DNS listener should be inert: %v, %+v", err, observer)
	} else {
		cleanup()
	}
	// A real UDP/TCP listener can bind to ephemeral ports; no privileged
	// port or external authoritative zone is needed for this lifecycle test.
	observer, cleanup, err := Start(ctx, testConfig())
	if err != nil {
		t.Fatal(err)
	}
	if observer == nil {
		t.Fatal("missing observer")
	}
	done := make(chan struct{})
	go func() {
		cleanup()
		cleanup()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("DNS listener shutdown hung")
	}
}

func TestNoCredentialsUsedByObserver(t *testing.T) {
	if v := os.Getenv("NETDIAG_DNS_UNUSED_CREDENTIAL"); v != "" {
		t.Skip("unexpected unrelated configuration")
	}
	if _, err := randomLabel(24); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPSChallengeHostLifecycle(t *testing.T) {
	observer := New(testConfig())
	session, err := observer.StartSession("192.0.2.77")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		host   string
		inZone bool
		active bool
	}{
		{session.Host, true, true},
		{strings.ToUpper(session.Host) + ".", true, true},
		{"observer.example.test", true, false},
		{"invalid.observer.example.test", true, false},
		{"nested." + session.Host, true, false},
		{"notobserver.example.test", false, false},
		{"example.test", false, false},
	}
	for _, tc := range cases {
		zone, active := observer.MatchHTTPSHost(tc.host)
		if zone != tc.inZone || active != tc.active {
			t.Errorf("host %q: got zone=%t active=%t; want zone=%t active=%t",
				tc.host, zone, active, tc.inZone, tc.active)
		}
	}
	observer.mu.Lock()
	observer.byID[session.ID].expires = time.Now().Add(-time.Second)
	observer.mu.Unlock()
	zone, active := observer.MatchHTTPSHost(session.Host)
	if !zone || active {
		t.Fatal("expired challenge should not serve a pixel")
	}
}
