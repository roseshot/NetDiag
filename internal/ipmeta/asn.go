package ipmeta

import (
	"net/netip"
	"os"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

const (
	DatabaseType = "GeoLite2-ASN"
	Source       = "MaxMind GeoLite2 ASN"
	checkEvery   = time.Minute
)

type Record struct {
	Number       uint   `maxminddb:"autonomous_system_number"`
	Organization string `maxminddb:"autonomous_system_organization"`
}

type Result struct {
	Status       string
	ASN          uint
	Organization string
	Network      string
	DatabaseTime string
}

type Store struct {
	path      string
	mu        sync.Mutex
	reader    *maxminddb.Reader
	file      os.FileInfo
	checkedAt time.Time
	buildTime string
}

// New creates a local database reader. An empty path disables ASN lookups.
func New(path string) *Store {
	return &Store{path: path}
}

// Close releases memory-mapped resources after the HTTP server stops.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reader == nil {
		return nil
	}
	err := s.reader.Close()
	s.reader = nil
	s.file = nil
	return err
}

// reloadLocked supports atomic MMDB replacement without restarting the server.
// The updater must not modify or truncate a memory-mapped database in place.
func (s *Store) reloadLocked() {
	if time.Since(s.checkedAt) < checkEvery {
		return
	}
	s.checkedAt = time.Now()
	info, err := os.Stat(s.path)
	if err != nil || info.IsDir() {
		s.dropLocked()
		return
	}
	if s.reader != nil && s.file != nil && os.SameFile(info, s.file) &&
		info.Size() == s.file.Size() && info.ModTime().Equal(s.file.ModTime()) {
		return
	}
	next, err := maxminddb.Open(s.path)
	if err != nil {
		s.dropLocked()
		return
	}
	if next.Metadata.DatabaseType != DatabaseType {
		_ = next.Close()
		s.dropLocked()
		return
	}
	if err := next.Verify(); err != nil {
		_ = next.Close()
		s.dropLocked()
		return
	}
	old := s.reader
	s.reader = next
	s.file = info
	if next.Metadata.BuildEpoch > 0 {
		s.buildTime = next.Metadata.BuildTime().UTC().Format(time.RFC3339)
	} else {
		s.buildTime = ""
	}
	if old != nil {
		_ = old.Close()
	}
}

func (s *Store) dropLocked() {
	if s.reader != nil {
		_ = s.reader.Close()
		s.reader = nil
	}
	s.file = nil
	s.buildTime = ""
}

// Lookup only uses a local MMDB; missing records never become invented ASNs.
func (s *Store) Lookup(ip netip.Addr) Result {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return Result{Status: "not_applicable"}
	}
	if s.path == "" {
		return Result{Status: "not_configured"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reloadLocked()
	if s.reader == nil {
		return Result{Status: "unavailable"}
	}

	var record Record
	result := s.reader.Lookup(ip)
	if err := result.Decode(&record); err != nil {
		return Result{Status: "unavailable"}
	}
	if record.Number == 0 && record.Organization == "" {
		return Result{Status: "not_found", DatabaseTime: s.buildTime}
	}
	network := ""
	if prefix := result.Prefix(); prefix.IsValid() {
		network = prefix.String()
	}
	return Result{
		Status:       "completed",
		ASN:          record.Number,
		Organization: record.Organization,
		Network:      network,
		DatabaseTime: s.buildTime,
	}
}
