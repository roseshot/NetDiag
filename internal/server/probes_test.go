package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateProbeOrigin(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"https://v4.example.test", "https://v4.example.test"},
		{"https://v6.example.test:8443", "https://v6.example.test:8443"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://127.0.0.1:8080", "http://127.0.0.1:8080"},
		{"http://[::1]:8080", "http://[::1]:8080"},
		{"http://v4.example.test", ""},
		{"https://v4.example.test/path", ""},
		{"https://v4.example.test?x=1", ""},
		{"https://v4.example.test#fragment", ""},
		{"https://user:pass@v4.example.test", ""},
		{"javascript:alert(1)", ""},
		{"https://v4.example.test\nother", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := validateProbeOrigin(tc.raw); got != tc.want {
			t.Errorf("validateProbeOrigin(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestAddressFamilyProbe(t *testing.T) {
	handler := New()
	cases := []struct {
		path   string
		peer   string
		status int
		family string
		ip     string
	}{
		{"/api/diag/address?family=4", "192.0.2.15:12345", 200, "IPv4", "192.0.2.15"},
		{"/api/diag/address?family=6", "[2001:db8::15]:12345", 200, "IPv6", "2001:db8::15"},
		{"/api/diag/address?family=6", "192.0.2.15:12345", 409, "", ""},
		{"/api/diag/address?family=4", "[2001:db8::15]:12345", 409, "", ""},
		{"/api/diag/address?family=7", "192.0.2.15:12345", 400, "", ""},
		{"/api/diag/address?family=4", "unknown", 503, "", ""},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		req.RemoteAddr = tc.peer
		req.Header.Set("X-Forwarded-For", "203.0.113.99")
		req.Header.Set("Forwarded", "for=203.0.113.99")
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Errorf("%s with %s: status %d, want %d", tc.path, tc.peer, res.Code, tc.status)
			continue
		}
		if tc.status == http.StatusOK {
			var body struct {
				Status string
				IP     string
				Family string
				Scope  string
			}
			if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Status != "completed" || body.Family != tc.family || body.IP != tc.ip ||
				body.Scope != "direct_peer" {
				t.Errorf("unexpected direct-peer probe response: %+v", body)
			}
		}
		if tc.status == http.StatusConflict && !strings.Contains(res.Body.String(), "address_family_mismatch") {
			t.Errorf("missing mismatch reason: %s", res.Body.String())
		}
		if tc.status == http.StatusConflict || tc.status == http.StatusServiceUnavailable {
			if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Errorf("error content type = %q", got)
			}
		}
	}
}

func TestConfiguredProbeOriginsAndCORS(t *testing.T) {
	t.Setenv("NETDIAG_IPV4_ORIGIN", "https://ipv4.example.test")
	t.Setenv("NETDIAG_IPV6_ORIGIN", "https://ipv6.example.test")
	t.Setenv("NETDIAG_PROBE_ALLOWED_ORIGINS", "https://netdiag.example.test")
	handler := New()

	configReq := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	configRes := httptest.NewRecorder()
	handler.ServeHTTP(configRes, configReq)
	var config struct {
		ProbeOrigins map[string]string
	}
	if err := json.Unmarshal(configRes.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if config.ProbeOrigins["ipv4"] != "https://ipv4.example.test" ||
		config.ProbeOrigins["ipv6"] != "https://ipv6.example.test" {
		t.Errorf("probe origins missing from config: %+v", config.ProbeOrigins)
	}
	csp := configRes.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "connect-src 'self' https://ipv4.example.test https://ipv6.example.test") {
		t.Errorf("CSP does not include explicitly configured origins: %s", csp)
	}

	for _, tc := range []struct {
		origin string
		want   string
	}{
		{"https://netdiag.example.test", "https://netdiag.example.test"},
		{"https://evil.example.test", ""},
		{"", ""},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/diag/address?family=4", nil)
		req.RemoteAddr = "192.0.2.10:1010"
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if got := res.Header().Get("Access-Control-Allow-Origin"); got != tc.want {
			t.Errorf("origin %q: allowed origin = %q, want %q", tc.origin, got, tc.want)
		}
		if res.Header().Get("Vary") != "Origin" {
			t.Errorf("missing Vary: Origin for %q", tc.origin)
		}
		if res.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Error("cross-origin probes must not enable credentials")
		}
	}
}

func TestInvalidOriginNotExposed(t *testing.T) {
	t.Setenv("NETDIAG_IPV4_ORIGIN", "https://invalid.example.test/path")
	t.Setenv("NETDIAG_IPV6_ORIGIN", "http://untrusted.example.test")
	t.Setenv("NETDIAG_PROBE_ALLOWED_ORIGINS", "https://untrusted.example.test/path")
	handler := New()
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var config struct {
		ProbeOrigins map[string]string
	}
	if err := json.Unmarshal(res.Body.Bytes(), &config); err != nil {
		t.Fatal(err)
	}
	if config.ProbeOrigins["ipv4"] != "" || config.ProbeOrigins["ipv6"] != "" {
		t.Errorf("invalid probe origins were exposed: %+v", config.ProbeOrigins)
	}
	if strings.Contains(res.Header().Get("Content-Security-Policy"), "invalid.example.test") {
		t.Fatal("invalid origin leaked into CSP")
	}
}
