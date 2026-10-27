# Architecture

## Component boundaries

- `cmd/netdiag`: starts the HTTP server and handles graceful shutdown.
- `internal/server`: exposes bounded, cache-disabled measurement endpoints, IP observation, independent IPv4/IPv6 probe endpoints with strict CORS and TCP diagnostic access.
- `internal/diagnostics`: Linux-specific TCP_INFO reading; other platforms report unavailable.
- `internal/ipmeta`: locally managed, startup-detected and background-hot-reloadable IPinfo or MaxMind ASN MMDB lookups with serialized reader switching.
- `internal/dnsdiag`: optional authoritative DNS service with random, expiring sessions and bounded recursive DNS peer observations.
- `internal/tcpwindow`: in-memory, per-peer, expiring TCP_INFO observation sessions; retains one baseline and the furthest end sample per observed TCP socket.
- `web`: embeds the static browser client in the Go binary.
- `web/assets/app.js`: runs lightweight homepage checks and starts bandwidth measurements only after user interaction.

The initial implementation intentionally does not include a database, cache service, accounts, third-party analytics, or advertising.

## Browser measurement flow

1. On page load, the browser calls `/api/ip` and `/api/diag/tcp`, checks dedicated IPv4/IPv6 origins when configured, and independently probes the configured public STUN endpoints using WebRTC.
2. ASN information comes from an automatically selected local IPinfo or GeoLite2 ASN MMDB, while DNS resolver observation remains opt-in and disabled unless authoritative DNS is configured.
3. After the user clicks **Start test**, the browser stops outstanding STUN probes and measures idle HTTP RTT (P50 and jitter).
4. JavaScript downloads streamed non-compressible data using 1, 4, 8, or 16 concurrent workers, while a separate loop measures loaded HTTP RTT.
5. JavaScript uploads pseudorandom test payloads via XHR with progress reporting, bounded parallelism, and final byte counts only from successful server responses. Each load phase can create an independent TCP observation token, tag transfers, and request the socket retransmission delta after completion.
6. Cancellation aborts active browser requests. The measured window uses elapsed monotonic time. Each server transfer also has a 30-second request context.

## Data integrity cautions

- The server's TCP_INFO scope changes when a reverse proxy terminates TCP. Direct Go HTTPS termination is supported through certificate file environment variables. TCP window counters use one baseline and the highest end sample per accepted socket; HTTP/2 multiplexing, non-exclusive sockets, kernel send buffers and timing mean deltas cannot be attributed precisely to one HTTP transfer or to the end-to-end browser path.
- IPv4/IPv6 probe endpoints validate the direct socket family and ignore forwarding headers. DNS and TLS routing must force the expected address family; a public terminating proxy prevents reliable peer observation.
- A stream is not a TCP connection when multiplexing is enabled.
- Browser-side upload progress is only used for the live display; final upload speed uses confirmed complete request payloads and can underestimate partially completed work.
- STUN server-reflexive candidates alone cannot determine a full NAT behavior class.
- Resolver observation needs independent authoritative DNS infrastructure to prevent reporting server-side DNS as client DNS.
- HTTP loaded latency (P50, P95 and P50-added latency relative to idle) is implemented as an initial diagnostic, not as a standardized bufferbloat grade. Some samples may be unavailable due to timeouts.

## Planned modules

- `internal/ipmeta`: implemented IPinfo and MaxMind ASN MMDB lookup with provider-specific schemas; future work includes verified announced BGP prefixes and stale-data monitoring.
- `internal/dnsdiag`: authoritative DNS challenge generation and resolver session correlation.
- `internal/tcpdiag`: dedicated client-facing TCP probe listener and more rigorous socket/session isolation for independently verified per-test counters.
- `internal/speedtest`: improved warm-up, automatic concurrency and calibrated measurement-window validation.
- `internal/monitoring`: rate controls, transfer budgets, structured metrics.

Do not present unimplemented or inferred values as direct measurements.

## Startup capability reporting

`internal/server/startup.go` builds the operator-facing startup summary from the initialized HTTP handler, ASN database status, validated dedicated address-family probe origins, and active DNS observer. It distinguishes server-available features from optional but unconfigured capabilities and browser-dependent STUN functionality. The summary is informational: a configured external probe origin is not proof of public reachability. A separate listener message identifies HTTP versus direct HTTPS termination.
