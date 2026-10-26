# Roadmap

This document distinguishes delivered scaffold behavior from future work.

## Implemented in the scaffold

- Go HTTP application and embedded responsive browser interface.
- Basic HTTP RTT/jitter probes.
- Single/multi-stream downloads and uploads (estimated upload metrics).
- Linux server-side TCP_INFO and direct-peer IP display.
- Configurable public WebRTC STUN probes with disabled-WebRTC fallback.
- Build/test workflow and Docker packaging.

## Planned, not yet implemented

1. Calibrated test engine with request/session accounting, upload confirmation, adaptive parallelism and loaded-latency tests.
2. Direct client-facing TCP diagnostic listener and proper per-test retransmission deltas.
3. IPv4/IPv6 dual-stack endpoints with separately verified addresses.
4. Licensed local IP-to-ASN lookup, country metadata and BGP prefix data if licensed.
5. Authoritative DNS resolver observer and precise TTL-bounded session correlation.
6. More granular WebRTC diagnostics, retaining `Undetermined` when NAT behavior cannot be established.
7. Optional explicit-activation controls for third-party probes.
8. Hardened rate limiting, observability and bounded traffic/cost policies.
9. Privacy policy, legal bases and EU deployment review.
10. Browser QA, integration testing and reproducible accuracy benchmarking.

No roadmap item is represented as a completed feature in the UI or API.
