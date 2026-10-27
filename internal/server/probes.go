package server

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
)

type probeConfig struct {
	ipv4Origin string
	ipv6Origin string
	allowed    map[string]struct{}
}

// Accept HTTPS origins, or loopback HTTP for local development.
func validateProbeOrigin(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 255 || strings.ContainsAny(raw, "\r\n\t ") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" || u.User != nil ||
		u.Opaque != "" || u.Path != "" || u.RawPath != "" ||
		u.RawQuery != "" || u.Fragment != "" || u.RawFragment != "" {
		return ""
	}
	if u.Scheme != "https" {
		host := u.Hostname()
		ip := net.ParseIP(host)
		if u.Scheme != "http" || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return ""
		}
	}
	return u.Scheme + "://" + u.Host
}

func loadProbeConfig() probeConfig {
	c := probeConfig{
		ipv4Origin: validateProbeOrigin(os.Getenv("NETDIAG_IPV4_ORIGIN")),
		ipv6Origin: validateProbeOrigin(os.Getenv("NETDIAG_IPV6_ORIGIN")),
		allowed:    make(map[string]struct{}),
	}
	for _, raw := range strings.Split(os.Getenv("NETDIAG_PROBE_ALLOWED_ORIGINS"), ",") {
		if origin := validateProbeOrigin(raw); origin != "" {
			c.allowed[origin] = struct{}{}
		}
	}
	return c
}

func (c probeConfig) allows(origin string) bool {
	_, ok := c.allowed[origin]
	return origin != "" && ok
}

func (c probeConfig) csp() string {
	sources := []string{"'self'"}
	if c.ipv4Origin != "" {
		sources = append(sources, c.ipv4Origin)
	}
	if c.ipv6Origin != "" && c.ipv6Origin != c.ipv4Origin {
		sources = append(sources, c.ipv6Origin)
	}
	return "default-src 'self'; script-src 'self'; style-src 'self'; connect-src " +
		strings.Join(sources, " ") +
		"; img-src 'self' data:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"
}

func probeResponse(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// address reports the actual TCP peer; untrusted forwarding headers are ignored.
func (a *app) address(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	requested := r.URL.Query().Get("family")
	if requested != "4" && requested != "6" {
		http.Error(w, "family must be 4 or 6", http.StatusBadRequest)
		return
	}
	ip := net.ParseIP(peerAddress(r.RemoteAddr))
	if ip == nil {
		probeResponse(w, http.StatusServiceUnavailable, map[string]any{
			"status": "unavailable",
			"reason": "peer_ip_unavailable",
		})
		return
	}
	family := "IPv6"
	actual := "6"
	if ip.To4() != nil {
		family = "IPv4"
		actual = "4"
		ip = ip.To4()
	}
	if requested != actual {
		probeResponse(w, http.StatusConflict, map[string]any{
			"status": "unavailable",
			"reason": "address_family_mismatch",
			"observedFamily": family,
			"scope": "direct_peer",
		})
		return
	}
	respondJSON(w, map[string]any{
		"status": "completed",
		"ip": ip.String(),
		"family": family,
		"scope": "direct_peer",
	})
}
