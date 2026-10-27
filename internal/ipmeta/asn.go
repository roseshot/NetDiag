package ipmeta

import (
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oschwald/maxminddb-golang/v2"
)

const (
	DatabaseType = "GeoLite2-ASN"
	Source = "MaxMind GeoLite2 ASN"
	SourceIPinfo = "IPinfo"
	checkEvery = time.Minute
)

var filenames = []string{
	"ipinfo_asn.mmdb",
	"ipinfo_lite.mmdb",
	"asn.mmdb",
	"country_asn.mmdb",
	"GeoLite2-ASN.mmdb",
}

type Record struct {
	Number uint `maxminddb:"autonomous_system_number"`
	Organization string `maxminddb:"autonomous_system_organization"`
}

type ipinfoRecord struct {
	ASN string `maxminddb:"asn"`
	Name string `maxminddb:"name"`
	Domain string `maxminddb:"domain"`
	ASName string `maxminddb:"as_name"`
	ASDomain string `maxminddb:"as_domain"`
}

type Result struct {
	Status string
	ASN uint
	Organization string
	Domain string
	Network string
	DatabaseTime string
	Source string
	DatabaseType string
}

type Store struct {
	mu sync.Mutex
	paths []string
	explicit bool
	reader *maxminddb.Reader
	file os.FileInfo
	selected string
	provider string
	dbType string
	buildTime string
	status string
	checkedAt time.Time
}

// New retains the explicit-path API; an empty path disables lookups.
func New(path string) *Store {
	if path == "" { return &Store{status: "not_configured"} }
	return &Store{paths: []string{path}, explicit: true, status: "unavailable"}
}

// NewAuto discovers available local MMDBs immediately at startup.
// An explicit file takes precedence over automatic directory discovery.
func NewAuto(explicitPath, directory string) *Store {
	var s *Store
	if explicitPath != "" {
		s = New(explicitPath)
	} else {
		if directory == "" { directory = "./data" }
		paths := make([]string, 0, len(filenames))
		for _, name := range filenames { paths = append(paths, filepath.Join(directory, name)) }
		s = &Store{paths: paths, status: "not_configured"}
	}
	s.Refresh()
	return s
}

// Refresh forces immediate redetection; regular lookups check once per minute.
func (s *Store) Refresh() Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checkedAt = time.Time{}
	s.detectLocked()
	return s.statusLocked()
}

func (s *Store) Status() Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detectLocked()
	return s.statusLocked()
}

func (s *Store) statusLocked() Result {
	return Result{Status: s.status, Source: s.provider, DatabaseType: s.dbType, DatabaseTime: s.buildTime}
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reader == nil { return nil }
	err := s.reader.Close()
	s.reader = nil
	s.file = nil
	s.selected = ""
	s.provider = ""
	s.status = "unavailable"
	return err
}

// Only recognized ASN-capable MMDB metadata types are accepted.
func providerFromMetadata(databaseType string) string {
	if databaseType == DatabaseType { return Source }
	t := strings.ToLower(databaseType)
	if strings.HasPrefix(t, "ipinfo ") && (strings.Contains(t, "asn") || strings.Contains(t, "lite")) {
		return SourceIPinfo
	}
	return ""
}

func parseIPinfoASN(value string) (uint, bool) {
	if len(value) < 3 || !strings.EqualFold(value[:2], "AS") { return 0, false }
	for _, ch := range value[2:] {
		if ch < '0' || ch > '9' { return 0, false }
	}
	number, err := strconv.ParseUint(value[2:], 10, 32)
	if err != nil || number == 0 { return 0, false }
	return uint(number), true
}

func (s *Store) detectLocked() {
	if time.Since(s.checkedAt) < checkEvery { return }
	s.checkedAt = time.Now()
	if len(s.paths) == 0 {
		s.dropLocked()
		s.status = "not_configured"
		return
	}
	seen := false
	for _, path := range s.paths {
		info, err := os.Stat(path)
		if err != nil || info.IsDir() { continue }
		seen = true
		if s.reader != nil && s.file != nil && s.selected == path &&
			os.SameFile(info, s.file) &&
			info.Size() == s.file.Size() &&
			info.ModTime().Equal(s.file.ModTime()) {
			s.status = "completed"
			return
		}
		next, err := maxminddb.Open(path)
		if err != nil { continue }
		provider := providerFromMetadata(next.Metadata.DatabaseType)
		if provider == "" {
			_ = next.Close()
			continue
		}
		if err := next.Verify(); err != nil {
			_ = next.Close()
			continue
		}
		old := s.reader
		s.reader = next
		s.file = info
		s.selected = path
		s.provider = provider
		s.dbType = next.Metadata.DatabaseType
		s.buildTime = ""
		if next.Metadata.BuildEpoch > 0 {
			s.buildTime = next.Metadata.BuildTime().UTC().Format(time.RFC3339)
		}
		s.status = "completed"
		if old != nil { _ = old.Close() }
		return
	}
	s.dropLocked()
	if s.explicit || seen { s.status = "unavailable" } else { s.status = "not_configured" }
}

func (s *Store) dropLocked() {
	if s.reader != nil {
		_ = s.reader.Close()
		s.reader = nil
	}
	s.file = nil
	s.selected = ""
	s.provider = ""
	s.dbType = ""
	s.buildTime = ""
}

// Lookup performs no external requests and reports absent data explicitly.
func (s *Store) Lookup(ip netip.Addr) Result {
	ip = ip.Unmap()
	if !ip.IsValid() || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return Result{Status: "not_applicable"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.detectLocked()
	if s.reader == nil { return Result{Status: s.status} }
	details := s.statusLocked()
	match := s.reader.Lookup(ip)
	if s.provider == Source {
		var record Record
		if err := match.Decode(&record); err != nil {
			details.Status = "unavailable"
			return details
		}
		if record.Number == 0 {
			details.Status = "not_found"
			return details
		}
		details.ASN = record.Number
		details.Organization = record.Organization
	} else {
		var record ipinfoRecord
		if err := match.Decode(&record); err != nil {
			details.Status = "unavailable"
			return details
		}
		if record.ASN == "" {
			details.Status = "not_found"
			return details
		}
		number, ok := parseIPinfoASN(record.ASN)
		if !ok {
			details.Status = "unavailable"
			return details
		}
		details.ASN = number
		details.Organization = record.ASName
		if details.Organization == "" { details.Organization = record.Name }
		details.Domain = record.ASDomain
		if details.Domain == "" { details.Domain = record.Domain }
	}
	if prefix := match.Prefix(); prefix.IsValid() {
		details.Network = prefix.String()
	}
	return details
}
