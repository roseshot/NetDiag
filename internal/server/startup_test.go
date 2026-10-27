package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/roseshot/NetDiag/internal/dnsdiag"
	"github.com/roseshot/NetDiag/internal/ipmeta"
)

func featureFor(t *testing.T, features []startupFeature, name string) startupFeature {
	t.Helper()
	for _, feature := range features {
		if feature.name == name {
			return feature
		}
	}
	t.Fatalf("startup feature %q not found", name)
	return startupFeature{}
}

func TestStartupFeaturesWithoutOptionalServices(t *testing.T) {
	store := ipmeta.NewAuto("", t.TempDir())
	defer store.Close()
	instance := &app{asn: store}
	features := startupFeatures(instance, "linux")

	for _, name := range []string{
		"HTTP API and static UI", "Speed test", "HTTP latency",
		"Public IP observation", "Address-family API", "TCP diagnostics",
	} {
		if feature := featureFor(t, features, name); feature.status != "enabled" {
			t.Errorf("%s: expected enabled, got %s", name, feature.status)
		}
	}

	for _, name := range []string{
		"ASN database", "Independent IPv4 probe", "Independent IPv6 probe",
		"DNS resolver discovery",
	} {
		if feature := featureFor(t, features, name); feature.status != "not configured" {
			t.Errorf("%s: expected not configured, got %s", name, feature.status)
		}
	}
	if feature := featureFor(t, features, "STUN/NAT discovery"); feature.status != "browser-dependent" {
		t.Errorf("STUN should not be presented as server verified: %+v", feature)
	}
	if feature := featureFor(t, startupFeatures(instance, "darwin"), "TCP diagnostics"); feature.status != "unavailable" {
		t.Errorf("non-Linux TCP diagnostics misreported: %+v", feature)
	}
}

func TestStartupFeaturesWithConfiguredProbesAndObserver(t *testing.T) {
	store := ipmeta.NewAuto("", t.TempDir())
	defer store.Close()
	observer := dnsdiag.New(dnsdiag.Config{Zone: "observe.example.test."})
	instance := &app{
		asn: store,
		probes: probeConfig{
			ipv4Origin: "https://ipv4.example.test",
			ipv6Origin: "https://ipv6.example.test",
		},
		dnsObserver: observer,
	}
	features := startupFeatures(instance, "linux")
	for _, name := range []string{"Independent IPv4 probe", "Independent IPv6 probe"} {
		feature := featureFor(t, features, name)
		if feature.status != "configured" || !strings.Contains(feature.detail, "reachability not verified") {
			t.Errorf("%s should not claim successful public reachability: %+v", name, feature)
		}
	}
	feature := featureFor(t, features, "DNS resolver discovery")
	if feature.status != "enabled" || !strings.Contains(feature.detail, "observe.example.test") {
		t.Errorf("DNS observer should be enabled only when present: %+v", feature)
	}
}

func TestStartupFeatureASNInvalidDatabase(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ipinfo_asn.mmdb"), []byte("invalid MMDB"), 0600); err != nil {
		t.Fatal(err)
	}
	store := ipmeta.NewAuto("", dir)
	defer store.Close()
	instance := &app{asn: store}
	feature := featureFor(t, startupFeatures(instance, "linux"), "ASN database")
	if feature.status != "unavailable" {
		t.Errorf("invalid database should not be reported as enabled: %+v", feature)
	}
}

func TestStartupFeatureFormatting(t *testing.T) {
	feature := startupFeature{name: "DNS resolver discovery", status: "enabled", detail: "authoritative observer"}
	line := feature.String()
	if !strings.Contains(line, "[enabled") || !strings.Contains(line, "DNS resolver discovery: authoritative observer") {
		t.Fatalf("unexpected startup feature log format: %q", line)
	}
}
