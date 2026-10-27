package server

import (
	"fmt"
	"log"
	"runtime"
	"strings"
)

// startupFeature describes what this process actually supports or has
// configured. Externally hosted probes are not claimed to be reachable.
type startupFeature struct {
	name   string
	status string
	detail string
}

func (f startupFeature) String() string {
	if f.detail == "" {
		return fmt.Sprintf("  [%-17s] %s", f.status, f.name)
	}
	return fmt.Sprintf("  [%-17s] %s: %s", f.status, f.name, f.detail)
}

func startupFeatures(a *app, platform string) []startupFeature {
	features := []startupFeature{
		{"HTTP API and static UI", "enabled", "health, configuration and Privacy Policy"},
		{"Speed test", "enabled", "download/upload; 1, 4, 8 or 16 concurrent requests"},
		{"HTTP latency", "enabled", "idle RTT, jitter and loaded latency"},
		{"Public IP observation", "enabled", "direct HTTP peer (not trusted proxy headers)"},
		{"Address-family API", "enabled", "checks observed IPv4 or IPv6 direct peers"},
	}

	if platform == "linux" {
		features = append(features, startupFeature{"TCP diagnostics", "enabled", "Linux TCP_INFO and per-phase retransmission deltas; server-side sockets only"})
	} else {
		features = append(features, startupFeature{"TCP diagnostics", "unavailable", "Linux TCP_INFO requires Linux"})
	}

	asn := a.asn.Status()
	switch asn.ReloadStatus {
	case "completed", "stale":
		detail := fmt.Sprintf("%s (%s); hot reload active", asn.Source, asn.DatabaseType)
		if asn.ReloadStatus == "stale" {
			detail += "; last verified snapshot"
		}
		features = append(features, startupFeature{"ASN database", "enabled", detail})
	case "unavailable":
		features = append(features, startupFeature{"ASN database", "unavailable", "no valid IPinfo or MaxMind MMDB loaded; queries still work"})
	default:
		features = append(features, startupFeature{"ASN database", "not configured", "no local MMDB; ASN fields are hidden"})
	}

	for _, probe := range []struct {
		name   string
		origin string
	}{
		{"Independent IPv4 probe", a.probes.ipv4Origin},
		{"Independent IPv6 probe", a.probes.ipv6Origin},
	} {
		if probe.origin == "" {
			features = append(features, startupFeature{probe.name, "not configured", "dedicated origin not set"})
		} else {
			features = append(features, startupFeature{probe.name, "configured", probe.origin + " (reachability not verified)"})
		}
	}

	if a.dnsObserver == nil {
		features = append(features, startupFeature{"DNS resolver discovery", "not configured", "authoritative observer disabled"})
	} else {
		features = append(features, startupFeature{"DNS resolver discovery", "enabled", "authoritative observer zone " + a.dnsObserver.Zone() + "; opt-in browser test"})
	}

	features = append(features, startupFeature{
		"STUN/NAT discovery", "browser-dependent",
		"WebRTC probes use third-party STUN servers; server cannot verify browser support",
	})
	return features
}

func logStartupFeatures(a *app) {
	features := startupFeatures(a, runtime.GOOS)
	var lines []string
	for _, feature := range features {
		lines = append(lines, feature.String())
	}
	log.Printf("NetDiag startup features:\n%s", strings.Join(lines, "\n"))
}
