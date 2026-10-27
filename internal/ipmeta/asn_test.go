package ipmeta

import (
	"context"
	"time"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func TestUnavailableStates(t *testing.T) {
	if got := New("").Lookup(netip.MustParseAddr("1.1.1.1")).Status; got != "not_configured" {
		t.Fatalf("empty database path: got %q", got)
	}
	if got := New(filepath.Join(t.TempDir(), "missing.mmdb")).Lookup(netip.MustParseAddr("8.8.8.8")).Status; got != "unavailable" {
		t.Fatalf("missing database file: got %q", got)
	}
}

func TestNonPublicAddresses(t *testing.T) {
	s := New("")
	for _, value := range []string{"127.0.0.1", "::1", "10.0.0.1", "192.168.1.1", "fc00::1", "fe80::1", "0.0.0.0"} {
		if got := s.Lookup(netip.MustParseAddr(value)).Status; got != "not_applicable" {
			t.Errorf("IP %s returned %q, want not_applicable", value, got)
		}
	}
}

func TestRejectCorruptDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "GeoLite2-ASN.mmdb")
	if err := os.WriteFile(path, []byte("not a MaxMind DB"), 0600); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if result := s.Lookup(netip.MustParseAddr("1.1.1.1")); result.Status != "unavailable" {
		t.Errorf("invalid DB returned %q", result.Status)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

// This optional integration test uses a locally licensed, operator-supplied
// GeoLite2 ASN database. The standard tests do not download or bundle data.
func TestOptionalRealDatabase(t *testing.T) {
	path := os.Getenv("NETDIAG_ASN_TEST_DB_PATH")
	if path == "" {
		t.Skip("No locally licensed test database configured")
	}
	s := New(path)
	defer s.Close()
	result := s.Lookup(netip.MustParseAddr("1.1.1.1"))
	if result.Status != "completed" && result.Status != "not_found" {
		t.Fatalf("unexpected status with real ASN database: %q", result.Status)
	}
}

func TestIPinfoASNParsing(t *testing.T) {
	for _, tc := range []struct {
		value  string
		number uint
		ok     bool
	}{
		{"AS13335", 13335, true},
		{"as64512", 64512, true},
		{"AS4294967295", 4294967295, true},
		{"", 0, false},
		{"13335", 0, false},
		{"AS0", 0, false},
		{"ASgarbage", 0, false},
		{"AS4294967296", 0, false},
	} {
		got, ok := parseIPinfoASN(tc.value)
		if got != tc.number || ok != tc.ok {
			t.Errorf("parseIPinfoASN(%q) = %d, %t; want %d, %t", tc.value, got, ok, tc.number, tc.ok)
		}
	}
}

func TestASNDatabaseTypeDetection(t *testing.T) {
	for _, tc := range []struct {
		kind     string
		provider string
	}{
		{DatabaseType, Source},
		{"ipinfo bundle_location_lite.mmdb", SourceIPinfo},
		{"ipinfo generic_country_free_country_asn.mmdb", SourceIPinfo},
		{"ipinfo generic_country_free_asn.mmdb", SourceIPinfo},
		{"ipinfo bundle_asn_asn.mmdb", SourceIPinfo},
		{"GeoLite2-City", ""},
		{"GeoLite2-Country", ""},
		{"ipinfo generic_country_free_country.mmdb", ""},
		{"unknown", ""},
	} {
		if got := providerFromMetadata(tc.kind); got != tc.provider {
			t.Errorf("metadata type %q resolved to %q; want %q", tc.kind, got, tc.provider)
		}
	}
}

func TestAutoDetectFilesAndStatus(t *testing.T) {
	dir := t.TempDir()
	s := NewAuto("", dir)
	if got := s.Status().Status; got != "not_configured" {
		t.Fatalf("empty directory: got %q", got)
	}
	if len(s.paths) != len(filenames) {
		t.Fatalf("got %d candidates, want %d", len(s.paths), len(filenames))
	}
	if filepath.Base(s.paths[0]) != "ipinfo_asn.mmdb" {
		t.Fatal("IPinfo ASN should be preferred if several databases are present")
	}
	if err := os.WriteFile(filepath.Join(dir, "ipinfo_lite.mmdb"), []byte("not mmdb"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := s.Refresh().Status; got != "unavailable" {
		t.Fatalf("corrupt auto-detected database: got %q, want unavailable", got)
	}
	if err := os.Remove(filepath.Join(dir, "ipinfo_lite.mmdb")); err != nil {
		t.Fatal(err)
	}
	if got := s.Refresh().Status; got != "not_configured" {
		t.Fatalf("removed database: got %q, want not_configured", got)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestExplicitDatabaseOverridesAutoDirectory(t *testing.T) {
	dir := t.TempDir()
	s := NewAuto(filepath.Join(dir, "chosen.mmdb"), dir)
	if len(s.paths) != 1 || filepath.Base(s.paths[0]) != "chosen.mmdb" {
		t.Fatalf("explicit override ignored: %+v", s.paths)
	}
	if got := s.Status().Status; got != "unavailable" {
		t.Fatalf("missing explicit file: got %q, want unavailable", got)
	}
}

func TestBackgroundHotReloadWithoutRequests(t *testing.T) {
	dir := t.TempDir()
	store := NewAuto("", dir)
	ctx, cancel := context.WithCancel(context.Background())
	done := store.Watch(ctx, 10*time.Millisecond)
	defer func() {
		cancel()
		<-done
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}()
	state := func() string {
		store.mu.Lock()
		defer store.mu.Unlock()
		return store.status
	}
	waitFor := func(expected string) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			if got := state(); got == expected {
				return
			}
			select {
			case <-deadline:
				t.Fatalf("background watcher did not reach %s; got %s", expected, state())
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	filename := filepath.Join(dir, "ipinfo_asn.mmdb")
	if err := os.WriteFile(filename, []byte("invalid MMDB"), 0600); err != nil {
		t.Fatal(err)
	}
	waitFor("unavailable")
	if err := os.Remove(filename); err != nil {
		t.Fatal(err)
	}
	waitFor("not_configured")
}

func TestWatchStopsOnContextCancellation(t *testing.T) {
	store := NewAuto("", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	done := store.Watch(ctx, 10*time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher did not stop after context cancellation")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}

// Set NETDIAG_ASN_TEST_DB_PATH to run real MMDB hot-swap checks. No vendor
// database or credentials are downloaded as part of the CI test suite.
func TestOptionalHotSwapWithRealDatabase(t *testing.T) {
	source := os.Getenv("NETDIAG_ASN_TEST_DB_PATH")
	if source == "" {
		t.Skip("No operator-supplied MMDB for hot-swap integration test")
	}
	original, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "GeoLite2-ASN.mmdb")
	install := func(data []byte) {
		t.Helper()
		temp := path + ".tmp"
		if err := os.WriteFile(temp, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(temp, path); err != nil {
			t.Fatal(err)
		}
	}
	install(original)
	store := NewAuto(path, "")
	defer store.Close()
	first := store.Lookup(netip.MustParseAddr("1.1.1.1"))
	if first.Status != "completed" && first.Status != "not_found" {
		t.Fatalf("valid database was not loaded: %+v", first)
	}

	install([]byte("corrupt replacement"))
	if after := store.Refresh(); after.ReloadStatus != "stale" || after.Source != first.Source {
		t.Fatalf("last verified database not retained after invalid replacement: %+v", after)
	}
	if usable := store.Lookup(netip.MustParseAddr("1.1.1.1")); usable.ReloadStatus != "stale" ||
		(usable.Status != "completed" && usable.Status != "not_found") {
		t.Fatalf("previously valid database should remain queryable: %+v", usable)
	}
	install(original)
	if after := store.Refresh(); after.ReloadStatus != "completed" {
		t.Fatalf("database did not recover after restored valid file: %+v", after)
	}
}
