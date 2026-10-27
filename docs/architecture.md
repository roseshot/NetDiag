# Architecture

## Component boundaries

- `cmd/netdiag`: starts the HTTP server and handles graceful shutdown.
- `internal/server`: exposes bounded, cache-disabled measurement endpoints, IP observation, independent IPv4/IPv6 probe endpoints with strict CORS and TCP diagnostic access.
- `internal/diagnostics`: Linux-specific TCP_INFO reading; other platforms report unavailable.
- `web`: embeds the static browser client in the Go binary.
- `web/assets/app.js`: runs lightweight homepage checks and starts bandwidth measurements only after user interaction.

The initial implementation intentionally does not include a database, cache service, accounts, third-party analytics, or advertising.

## Browser measurement flow

1. On page load, the browser calls `/api/ip` and `/api/diag/tcp`, checks dedicated IPv4/IPv6 origins when configured, and independently probes the configured public STUN endpoints using WebRTC.
2. ASN and DNS resolver cards remain explicitly unconfigured.
3. After the user clicks **Start test**, the browser stops outstanding STUN probes and measures idle HTTP RTT (P50 and jitter).
4. JavaScript downloads streamed non-compressible data using 1, 4, 8, or 16 concurrent workers, while a separate loop measures loaded HTTP RTT.
5. JavaScript uploads pseudorandom test payloads via XHR with progress reporting, bounded parallelism, and final byte counts only from successful server responses.
6. Cancellation aborts active browser requests. The measured window uses elapsed monotonic time. Each server transfer also has a 30-second request context.

## Data integrity cautions

- The server's TCP_INFO scope changes when a reverse proxy terminates TCP. Direct Go HTTPS termination is supported through certificate file environment variables.
- IPv4/IPv6 probe endpoints validate the direct socket family and ignore forwarding headers. DNS and TLS routing must force the expected address family; a public terminating proxy prevents reliable peer observation.
- A stream is not a TCP connection when multiplexing is enabled.
- Browser-side upload progress is only used for the live display; final upload speed uses confirmed complete request payloads and can underestimate partially completed work.
- STUN server-reflexive candidates alone cannot determine a full NAT behavior class.
- Resolver observation needs independent authoritative DNS infrastructure to prevent reporting server-side DNS as client DNS.
- HTTP loaded latency (P50, P95 and P50-added latency relative to idle) is implemented as an initial diagnostic, not as a standardized bufferbloat grade. Some samples may be unavailable due to timeouts.

## Planned modules

- `internal/ipmeta`: licensed local IP-to-ASN metadata lookup with source attribution.
- `internal/dnsdiag`: authoritative DNS challenge generation and resolver session correlation.
- `internal/tcpdiag`: dedicated direct-to-client TCP probe listener with per-session retransmission deltas.
- `internal/speedtest`: improved warm-up, automatic concurrency and calibrated measurement-window validation.
- `internal/monitoring`: rate controls, transfer budgets, structured metrics.

Do not present unimplemented or inferred values as direct measurements.
