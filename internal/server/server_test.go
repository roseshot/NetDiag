package server

import (
	"context"
	"time"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoreEndpoints(t *testing.T) {
	handler := New()
	for _, tc := range []struct {
		path   string
		status int
	}{
		{"/api/health", 200},
		{"/api/config", 200},
		{"/api/ping", 204},
		{"/api/download?bytes=4096", 200},
		{"/api/download?bytes=0", 400},
		{"/api/ip", 200},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != tc.status {
			t.Errorf("%s: got %d, want %d", tc.path, res.Code, tc.status)
		}
	}
}

func TestDownloadSizeAndHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/download?bytes=1000", nil)
	res := httptest.NewRecorder()
	New().ServeHTTP(res, req)
	if got := len(res.Body.Bytes()); got != 1000 {
		t.Fatalf("received %d bytes, want 1000", got)
	}
	if res.Header().Get("Cache-Control") != "no-store, no-transform" {
		t.Fatal("missing no-store response header")
	}
}

func TestUpload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/upload", bytes.NewReader(make([]byte, 1024)))
	res := httptest.NewRecorder()
	New().ServeHTTP(res, req)
	if res.Code != 200 {
		t.Fatalf("upload status: %d", res.Code)
	}
	var response struct {
		ReceivedBytes int64 `json:"receivedBytes"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ReceivedBytes != 1024 {
		t.Fatalf("received %d bytes", response.ReceivedBytes)
	}
}

func TestRejectLargeUpload(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/upload", bytes.NewReader(make([]byte, maxUpload+1)))
	res := httptest.NewRecorder()
	New().ServeHTTP(res, req)
	if res.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("got %d, want 413", res.Code)
	}
}

func TestMethodValidation(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/ping", nil)
	res := httptest.NewRecorder()
	New().ServeHTTP(res, req)
	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("got %d, want 405", res.Code)
	}
}

func TestPrivacyPolicyIsPublic(t *testing.T) {
	handler := New()
	for _, tc := range []struct {
		path     string
		expected string
	}{
		{"/", `href="/privacy.html">Privacy Policy</a>`},
		{"/privacy.html", "<h1>Privacy Policy</h1>"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		if res.Code != http.StatusOK {
			t.Fatalf("%s: got status %d, want 200", tc.path, res.Code)
		}
		if !strings.Contains(res.Body.String(), tc.expected) {
			t.Errorf("%s: missing expected policy content", tc.path)
		}
	}
}

func TestPerPeerTransferQuota(t *testing.T) {
	a := &app{activePeers: make(map[string]int)}
	const peer = "2001:db8::1"
	for i := 0; i < maxPeerTransfers; i++ {
		if !a.acquirePeer(peer) {
			t.Fatalf("unexpected refusal at transfer %d", i)
		}
	}
	if a.acquirePeer(peer) {
		t.Fatal("per-peer quota was not enforced")
	}
	if !a.acquirePeer("192.0.2.1") {
		t.Fatal("unrelated peer should remain eligible")
	}
	for i := 0; i < maxPeerTransfers; i++ {
		a.releasePeer(peer)
	}
	a.releasePeer("192.0.2.1")
	if len(a.activePeers) != 0 {
		t.Fatal("peer state must be cleaned up after releases")
	}
}

func TestPeerAddress(t *testing.T) {
	for input, expected := range map[string]string{
		"192.0.2.5:1234":      "192.0.2.5",
		"[2001:db8::5]:1234":  "2001:db8::5",
		"unparseable-address": "unparseable-address",
	} {
		if actual := peerAddress(input); actual != expected {
			t.Errorf("peerAddress(%q) = %q, want %q", input, actual, expected)
		}
	}
}

func TestASNLookupStatuses(t *testing.T) {
	for _, tc := range []struct {
		name     string
		path     string
		remote   string
		expected string
	}{
		{name: "without_database", path: "", remote: "1.1.1.1:1234", expected: "not_configured"},
		{name: "missing_database", path: "/path/that/does/not/exist/GeoLite2-ASN.mmdb", remote: "1.1.1.1:1234", expected: "unavailable"},
		{name: "private_ip", path: "", remote: "10.0.0.1:1234", expected: "not_applicable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("NETDIAG_ASN_DB_PATH", tc.path)
			t.Setenv("NETDIAG_ASN_DB_DIR", t.TempDir())
			req := httptest.NewRequest(http.MethodGet, "/api/ip", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-For", "8.8.8.8")
			res := httptest.NewRecorder()
			New().ServeHTTP(res, req)
			if res.Code != http.StatusOK {
				t.Fatalf("got HTTP %d", res.Code)
			}
			var reply struct {
				IP        string `json:"ip"`
				ASN       *uint  `json:"asn"`
				ASNStatus string `json:"asnStatus"`
				ASNSource string `json:"asnSource"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &reply); err != nil {
				t.Fatal(err)
			}
			if reply.IP != peerAddress(tc.remote) || reply.ASNStatus != tc.expected || reply.ASN != nil {
				t.Errorf("unexpected ASN lookup response: %+v", reply)
			}
			if reply.ASNSource != "" {
				t.Errorf("unconfigured database must not claim a provider: %q", reply.ASNSource)
			}
		})
	}
}

func TestGeoLiteAttributionOnHomePage(t *testing.T) {
	res := httptest.NewRecorder()
	New().ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("homepage returned %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "This product includes GeoLite Data created by MaxMind") {
		t.Fatal("homepage missing MaxMind attribution")
	}
}

func TestASNBackgroundWatcherCleanup(t *testing.T) {
	t.Setenv("NETDIAG_ASN_DB_DIR", t.TempDir())
	t.Setenv("NETDIAG_ASN_DB_PATH", "")
	t.Setenv("NETDIAG_ASN_RELOAD_INTERVAL", "1s")
	ctx, cancel := context.WithCancel(context.Background())
	handler, cleanup := NewWithContext(ctx)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("server unavailable: got HTTP %d", res.Code)
	}
	cancel()
	done := make(chan struct{})
	go func() {
		cleanup()
		cleanup() // Cleanup must be idempotent.
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("ASN watcher cleanup did not complete")
	}
}

func TestASNReloadInterval(t *testing.T) {
	for _, tc := range []struct {
		value string
		expected time.Duration
	}{
		{"", 5 * time.Second},
		{"2s", 2 * time.Second},
		{"100ms", 5 * time.Second},
		{"bad", 5 * time.Second},
		{"2h", 5 * time.Second},
	} {
		t.Setenv("NETDIAG_ASN_RELOAD_INTERVAL", tc.value)
		if got := asnReloadInterval(); got != tc.expected {
			t.Errorf("reload interval %q: got %s, want %s", tc.value, got, tc.expected)
		}
	}
}
