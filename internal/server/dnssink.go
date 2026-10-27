package server

import (
	"net"
	"net/http"
	"strconv"
	"strings"
)

// This fixed 1x1 transparent GIF contains no visitor-specific data.
var dnsProbeGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00,
	0x01, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0xff,
	0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00,
	0x01, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01,
	0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01,
	0x00, 0x3b,
}

func requestHostname(authority string) string {
	if strings.Contains(authority, ":") {
		host, _, err := net.SplitHostPort(authority)
		if err != nil {
			return ""
		}
		authority = host
	}
	return strings.ToLower(strings.TrimSuffix(authority, "."))
}

// serveDNSHTTPSPixel is deliberately restricted to the observation zone.
// Unknown/expired challenge hosts cannot expose the main website or API.
func serveDNSHTTPSPixel(w http.ResponseWriter, r *http.Request, active bool) {
	if !active || r.URL.Path != "/pixel.gif" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Content-Length", strconv.Itoa(len(dnsProbeGIF)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(dnsProbeGIF)
	}
}
