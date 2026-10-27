package ipmeta

import (
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
