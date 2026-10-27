package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/roseshot/NetDiag/internal/diagnostics"
	"github.com/roseshot/NetDiag/internal/ipmeta"
	"github.com/roseshot/NetDiag/web"
)

const (
	maxDownload      = 16 << 20
	maxUpload        = 16 << 20
	defaultSize      = 4 << 20
	maxPeerTransfers = 20
)

type connectionKey struct{}

// ConnContext associates the accepted socket with requests on that connection.
// TCP_INFO describes this socket, not necessarily the browser when a reverse
// proxy terminates the public connection.
func ConnContext(ctx context.Context, conn net.Conn) context.Context {
	return context.WithValue(ctx, connectionKey{}, conn)
}

type app struct {
	slots       chan struct{}
	block       []byte
	peerMu      sync.Mutex
	activePeers map[string]int
	probes      probeConfig
	asn         *ipmeta.Store
}

func peerAddress(remote string) string {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		return remote
	}
	return host
}

func (a *app) acquirePeer(peer string) bool {
	a.peerMu.Lock()
	defer a.peerMu.Unlock()
	if a.activePeers[peer] >= maxPeerTransfers {
		return false
	}
	a.activePeers[peer]++
	return true
}

func (a *app) releasePeer(peer string) {
	a.peerMu.Lock()
	defer a.peerMu.Unlock()
	a.activePeers[peer]--
	if a.activePeers[peer] <= 0 {
		delete(a.activePeers, peer)
	}
}

func New() http.Handler {
	handler, _ := newHandler()
	return handler
}

// NewWithContext starts a background ASN database watcher for the running
// HTTP service. The returned cleanup function stops the watcher, waits for
// it to exit, and closes the MMDB reader.
func NewWithContext(ctx context.Context) (http.Handler, func()) {
	handler, database := newHandler()
	watchCtx, cancel := context.WithCancel(ctx)
	done := database.Watch(watchCtx, asnReloadInterval())
	var once sync.Once
	closeASN := func() {
		once.Do(func() {
			cancel()
			<-done
			if err := database.Close(); err != nil {
				log.Printf("ASN database close: %v", err)
			}
		})
	}
	return handler, closeASN
}

func asnReloadInterval() time.Duration {
	const fallback = 5 * time.Second
	raw := os.Getenv("NETDIAG_ASN_RELOAD_INTERVAL")
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < time.Second || value > time.Hour {
		log.Printf("Invalid NETDIAG_ASN_RELOAD_INTERVAL %q; using %s", raw, fallback)
		return fallback
	}
	return value
}

func newHandler() (http.Handler, *ipmeta.Store) {
	asn := ipmeta.NewAuto(os.Getenv("NETDIAG_ASN_DB_PATH"), os.Getenv("NETDIAG_ASN_DB_DIR"))
	selected := asn.Status()
	log.Printf("ASN database: status=%s source=%q type=%q", selected.Status, selected.Source, selected.DatabaseType)
	a := &app{slots: make(chan struct{}, 64), block: make([]byte, 32<<10), activePeers: make(map[string]int), probes: loadProbeConfig(), asn: asn}
	if _, err := rand.Read(a.block); err != nil {
		panic(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", a.health)
	mux.HandleFunc("/api/config", a.config)
	mux.HandleFunc("/api/ping", a.ping)
	mux.HandleFunc("/api/ip", a.ip)
	mux.HandleFunc("/api/diag/tcp", a.tcp)
	mux.HandleFunc("/api/diag/address", a.address)
	mux.HandleFunc("/api/download", a.transfer(a.download))
	mux.HandleFunc("/api/upload", a.transfer(a.upload))
	mux.Handle("/", http.FileServer(http.FS(web.Files())))
	return securityHeaders(mux, a.probes), asn
}

func securityHeaders(next http.Handler, probes probeConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Cache-Control", "no-store, no-transform")
		w.Header().Set("Content-Security-Policy", probes.csp())
		if r.URL.Path == "/api/diag/address" {
			w.Header().Add("Vary", "Origin")
			if origin := r.Header.Get("Origin"); probes.allows(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}

func respondJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(value)
}

func (a *app) health(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	respondJSON(w, map[string]string{"status": "ok"})
}

func (a *app) config(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	respondJSON(w, map[string]any{
		"name":                 "NetDiag",
		"modes":                []string{"single", "multi"},
		"streamOptions":        []int{1, 4, 8, 16},
		"defaultStreams":       4,
		"durationSeconds":      8,
		"downloadRequestBytes": defaultSize,
		"uploadRequestBytes":   defaultSize,
		"maxPeerTransfers":     maxPeerTransfers,
		"probeOrigins": map[string]string{
			"ipv4": a.probes.ipv4Origin,
			"ipv6": a.probes.ipv6Origin,
		},
		"tcpDiagnostics": "server_side",
		"dnsResolvers":   "not_configured",
		"udpNat":         "not_configured",
	})
}

func (a *app) ping(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) ip(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	host := peerAddress(r.RemoteAddr)
	parsed := net.ParseIP(host)
	family := "unknown"
	if parsed != nil {
		if parsed.To4() != nil {
			family = "IPv4"
		} else {
			family = "IPv6"
		}
	}
	var addr netip.Addr
	if parsed != nil {
		if actual, ok := netip.AddrFromSlice(parsed); ok {
			addr = actual.Unmap()
		}
	}
	info := a.asn.Lookup(addr)
	var number any
	if info.Status == "completed" {
		number = info.ASN
	}
	respondJSON(w, map[string]any{
		"ip":                 host,
		"family":             family,
		"asn":                number,
		"asnStatus":          info.Status,
		"asnOrganization":    info.Organization,
		"asnDomain":          info.Domain,
		"asnNetwork":         info.Network,
		"asnDatabaseBuiltAt": info.DatabaseTime,
		"asnSource":          info.Source,
		"asnDatabaseType":    info.DatabaseType,
		"asnReloadStatus":    info.ReloadStatus,
		"scope":              "direct_peer",
	})
}

func (a *app) tcp(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	conn, ok := r.Context().Value(connectionKey{}).(net.Conn)
	if !ok {
		respondJSON(w, map[string]any{"status": "unavailable", "reason": "connection_context_unavailable"})
		return
	}
	stats, err := diagnostics.TCPInfo(conn)
	if err != nil {
		respondJSON(w, map[string]any{"status": "unavailable", "reason": "tcp_info_unavailable"})
		return
	}
	respondJSON(w, stats)
}

func (a *app) transfer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case a.slots <- struct{}{}:
			defer func() { <-a.slots }()
		default:
			http.Error(w, "server busy", http.StatusTooManyRequests)
			return
		}
		peer := peerAddress(r.RemoteAddr)
		if !a.acquirePeer(peer) {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "too many active transfers for this peer", http.StatusTooManyRequests)
			return
		}
		defer a.releasePeer(peer)
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		next(w, r.WithContext(ctx))
	}
}

func (a *app) download(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodGet) {
		return
	}
	size := int64(defaultSize)
	if raw := r.URL.Query().Get("bytes"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n <= 0 || n > maxDownload {
			http.Error(w, "bytes must be between 1 and 16777216", http.StatusBadRequest)
			return
		}
		size = n
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("X-Accel-Buffering", "no")
	for remaining := size; remaining > 0; {
		if r.Context().Err() != nil {
			return
		}
		chunk := int64(len(a.block))
		if remaining < chunk {
			chunk = remaining
		}
		n, err := w.Write(a.block[:int(chunk)])
		remaining -= int64(n)
		if err != nil || n == 0 {
			return
		}
	}
}

func (a *app) upload(w http.ResponseWriter, r *http.Request) {
	if !requireMethod(w, r, http.MethodPost) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUpload)
	defer r.Body.Close()
	n, err := io.Copy(io.Discard, r.Body)
	if err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			http.Error(w, "upload exceeds 16 MiB limit", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "upload failed", http.StatusBadRequest)
		return
	}
	respondJSON(w, map[string]any{"receivedBytes": n})
}
