// Package tcpwindow collects TCP_INFO retransmission counter changes for
// sockets seen by explicitly tagged speed-test transfer requests.
package tcpwindow

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"net"
	"sync"
	"time"
)

const (
	sessionTTL = 60 * time.Second
	maxSessions = 512
	maxSessionsPerPeer = 4
	maxSocketsPerSession = 32
)

var ErrLimit = errors.New("TCP observation session limit reached")

type Sample func(net.Conn) (uint32, error)

type socketStats struct {
	baseline uint32
	delta uint32
	finished bool
}

type session struct {
	owner string
	expires time.Time
	sockets map[net.Conn]*socketStats
	active int
	completed int
	failures int
}

type Result struct {
	Status string `json:"status"`
	Retransmissions uint64 `json:"retransmissions"`
	Connections int `json:"connections"`
	CompletedRequests int `json:"completedRequests"`
	ActiveRequests int `json:"activeRequests"`
	FailedSamples int `json:"failedSamples"`
	Scope string `json:"scope"`
	Note string `json:"note"`
}

type Manager struct {
	mu sync.Mutex
	sessions map[string]*session
	sample Sample
	now func() time.Time
}

func New(sample Sample) *Manager {
	return &Manager{sessions: make(map[string]*session), sample: sample, now: time.Now}
}

func (m *Manager) purgeLocked() {
	now := m.now()
	for token, current := range m.sessions {
		if !now.Before(current.expires) {
			delete(m.sessions, token)
		}
	}
}

func newToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b), nil
}

func (m *Manager) Start(owner string) (string, time.Time, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked()
	if len(m.sessions) >= maxSessions {
		return "", time.Time{}, ErrLimit
	}
	peerSessions := 0
	for _, s := range m.sessions {
		if s.owner == owner { peerSessions++ }
	}
	if peerSessions >= maxSessionsPerPeer { return "", time.Time{}, ErrLimit }
	token, err := newToken()
	if err != nil { return "", time.Time{}, err }
	expires := m.now().Add(sessionTTL)
	m.sessions[token] = &session{owner: owner, expires: expires, sockets: make(map[net.Conn]*socketStats)}
	return token, expires, nil
}

// Begin returns a completion callback only for valid session + socket pairs.
// The baseline is sampled once per physical socket even if multiple HTTP
// requests multiplex over it. Each finish records the highest observed delta;
// it never adds overlapping per-request changes.
func (m *Manager) Begin(token, owner string, conn net.Conn) func() {
	if conn == nil || token == "" || len(token) != 39 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked()
	s := m.sessions[token]
	if s == nil || s.owner != owner { return nil }
	if m.sample == nil {
		s.failures++
		return nil
	}
	stats, ok := s.sockets[conn]
	if !ok {
		if len(s.sockets) >= maxSocketsPerSession {
			s.failures++
			return nil
		}
		before, err := m.sample(conn)
		if err != nil {
			s.failures++
			return nil
		}
		stats = &socketStats{baseline: before}
		s.sockets[conn] = stats
	}
	s.active++
	return func() {
		after, err := m.sample(conn)
		m.mu.Lock()
		defer m.mu.Unlock()
		s.active--
		s.completed++
		if err != nil {
			s.failures++
			return
		}
		// TCP_INFO counters are uint32 and may wrap around. A single window
		// is capped at one minute; modulo subtraction handles a single wrap.
		delta := after - stats.baseline
		if !stats.finished || delta > stats.delta { stats.delta = delta }
		stats.finished = true
	}
}

// Read returns one phase's snapshot and consumes the token if no requests are
// active. This limits token reuse and retention; the browser reads after work.
func (m *Manager) Read(token, owner string) (Result, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeLocked()
	s := m.sessions[token]
	if s == nil || s.owner != owner { return Result{}, false }
	result := Result{
		Scope: "server_tcp_sockets",
		CompletedRequests: s.completed,
		ActiveRequests: s.active,
		FailedSamples: s.failures,
		Note: "TCP_INFO totals describe Go-accepted sockets, including shared HTTP/2 traffic or proxy-to-Go connections; this is not an end-to-end packet loss rate.",
	}
	for _, sock := range s.sockets {
		if sock.finished {
			result.Connections++
			result.Retransmissions += uint64(sock.delta)
		}
	}
	switch {
	case s.active > 0:
		result.Status = "pending"
	case result.Connections == 0 || s.failures > 0:
		result.Status = "unavailable"
	default:
		result.Status = "completed"
	}
	if s.active == 0 { delete(m.sessions, token) }
	return result, true
}
