package server

import (
	"bytes"
	"image/gif"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/roseshot/NetDiag/internal/dnsdiag"
)

func TestDNSHTTPSProbeOnlyForActiveChallenge(t *testing.T) {
	t.Setenv("NETDIAG_ASN_DB_PATH", "")
	t.Setenv("NETDIAG_ASN_DB_DIR", t.TempDir())
	observer := dnsdiag.New(dnsdiag.Config{Zone: "observe.example.test."})
	session, err := observer.StartSession("192.0.2.55")
	if err != nil {
		t.Fatal(err)
	}
	handler, _ := newHandler(observer)
	probe := func(method, host, path string) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "https://"+host+path, nil)
		request.Host = host
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
	for _, host := range []string{session.Host, strings.ToUpper(session.Host), session.Host + ":443"} {
		got := probe(http.MethodGet, host, "/pixel.gif")
		if got.Code != http.StatusOK {
			t.Errorf("valid challenge host %q: HTTP %d: %s", host, got.Code, got.Body.String())
			continue
		}
		if got.Header().Get("Content-Type") != "image/gif" {
			t.Errorf("unexpected probe type: %s", got.Header().Get("Content-Type"))
		}
		if got.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("challenge pixel must not be cached")
		}
		if got.Header().Get("Content-Length") != strconv.Itoa(len(dnsProbeGIF)) {
			t.Fatal("wrong response content length")
		}
		info, err := gif.DecodeConfig(bytes.NewReader(got.Body.Bytes()))
		if err != nil || info.Width != 1 || info.Height != 1 {
			t.Errorf("invalid one-pixel GIF: %+v, %v", info, err)
		}
	}
	if got := probe(http.MethodHead, session.Host, "/pixel.gif"); got.Code != http.StatusOK || got.Body.Len() != 0 {
		t.Errorf("HEAD pixel status %d, body length %d", got.Code, got.Body.Len())
	}
	if got := probe(http.MethodPost, session.Host, "/pixel.gif"); got.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST pixel status %d, expected 405", got.Code)
	}

	for _, host := range []string{
		"unknown.observe.example.test",
		"nested." + session.Host,
		"observe.example.test",
	} {
		for _, path := range []string{"/pixel.gif", "/", "/api/health", "/api/ip"} {
			if got := probe(http.MethodGet, host, path); got.Code != http.StatusNotFound {
				t.Errorf("unexpected exposure on %s%s: HTTP %d", host, path, got.Code)
			}
		}
	}
	if got := probe(http.MethodGet, session.Host, "/api/health"); got.Code != http.StatusNotFound {
		t.Errorf("main API exposed on challenge host: HTTP %d", got.Code)
	}
	if got := probe(http.MethodGet, "netdiag.example.test", "/api/health"); got.Code != http.StatusOK {
		t.Errorf("main website must remain available, got %d", got.Code)
	}
}

func TestDNSHTTPProbeHostnameParsing(t *testing.T) {
	for _, item := range []struct {
		host string
		want string
	}{
		{"Hello.Example.Test", "hello.example.test"},
		{"hello.example.test.", "hello.example.test"},
		{"Hello.Example.Test:443", "hello.example.test"},
		{"[::1]:443", "::1"},
		{"Hello.Example.Test:badport", "hello.example.test"},
		{"Hello.Example.Test:443:bad", ""},
	} {
		if got := requestHostname(item.host); got != item.want {
			t.Errorf("hostname %q: got %q, want %q", item.host, got, item.want)
		}
	}
}
