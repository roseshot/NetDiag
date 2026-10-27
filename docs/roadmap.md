# Roadmap

This document distinguishes delivered scaffold behavior from future work.

## Implemented in the scaffold

- Go HTTP application and embedded responsive browser interface.
- Idle HTTP RTT/jitter probes and load-time RTT measurements (P50, P95 and added latency).
- Single/multi-stream downloads and uploads, with final upload bytes verified by server acknowledgement.
- Linux server-side TCP_INFO and direct-peer IP display.
- Optional MaxMind GeoLite2 ASN MMDB lookups with database type/integrity validation, automatic file-change detection, attribution and safe unavailable states.
- Configurable IPv4/IPv6 independent probe origins, address-family verification, limited cross-origin access and browser HTTP RTT checks.
- Optional direct Go HTTPS listener using externally managed certificates.
- Configurable public WebRTC STUN probes with disabled-WebRTC fallback.
- Per-peer request concurrency caps in addition to the global transfer limit.
- Browser measurement math tests and Go HTTP tests.
- Build/test workflow and Docker packaging.

## Planned, not yet implemented

1. Calibrated transfer window with warm-up, session accounting, in-flight upload byte reconciliation, adaptive parallelism and reference benchmark validation.
2. Direct client-facing TCP diagnostic listener and proper per-test retransmission deltas.
3. Production dual-stack deployments and cross-network browser validation of independent IPv4/IPv6 probe origins.
4. Production MaxMind data freshness alerting and independent BGP prefix verification (GeoLite matching ranges are not announced BGP prefixes).
5. Authoritative DNS resolver observer and precise TTL-bounded session correlation.
6. More granular WebRTC diagnostics, retaining `Undetermined` when NAT behavior cannot be established.
7. Optional explicit-activation controls for third-party probes.
8. Hardened rate limiting, observability and bounded traffic/cost policies.
9. Final operator-specific Privacy Policy and deployment privacy review.
10. Browser QA, integration testing and reproducible accuracy benchmarking.

No roadmap item is represented as a completed feature in the UI or API.
