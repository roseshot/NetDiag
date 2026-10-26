# Architecture

## Component boundaries

- `cmd/netdiag`: starts the HTTP server and handles graceful shutdown.
- `internal/server`: exposes bounded, cache-disabled measurement endpoints, IP observation, and TCP diagnostic access.
- `internal/diagnostics`: Linux-specific TCP_INFO reading; other platforms report unavailable.
- `web`: embeds the static browser client in the Go binary.
- `web/assets/app.js`: runs lightweight homepage checks and starts bandwidth measurements only after user interaction.

The initial implementation intentionally does not include a database, cache service, accounts, third-party analytics, or advertising.

## Browser measurement flow

1. On page load, the browser calls `/api/ip` and `/api/diag/tcp`, and independently probes the configured public STUN endpoints using WebRTC.
2. ASN and DNS resolver cards remain explicitly unconfigured.
3. After the user clicks **Start test**, JavaScript measures HTTP RTT samples.
4. JavaScript downloads streamed non-compressible data using 1, 4, 8, or 16 concurrent workers.
5. JavaScript uploads pseudorandom test payloads via XHR with progress reporting and bounded parallelism.
6. Cancellation aborts active browser requests. Each server transfer also has a 30-second request context.

## Data integrity cautions

- The server's TCP_INFO scope changes when a reverse proxy terminates TCP.
- A stream is not a TCP connection when multiplexing is enabled.
- Browser-side upload progress is not the same as bytes confirmed received by the server.
- STUN server-reflexive candidates alone cannot determine a full NAT behavior class.
- Resolver observation needs independent authoritative DNS infrastructure to prevent reporting server-side DNS as client DNS.
- For loaded latency/bufferbloat, measure idle, download-loaded and upload-loaded HTTP RTT with consistent baselines; this is not yet implemented.

## Planned modules

- `internal/ipmeta`: licensed local IP-to-ASN metadata lookup with source attribution.
- `internal/dnsdiag`: authoritative DNS challenge generation and resolver session correlation.
- `internal/tcpdiag`: dedicated direct-to-client TCP probe listener with per-session retransmission deltas.
- `internal/speedtest`: calibrated windows, warm-up, automatic concurrency and bufferbloat analysis.
- `internal/monitoring`: rate controls, transfer budgets, structured metrics.

Do not present unimplemented or inferred values as direct measurements.
