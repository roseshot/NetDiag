# NetDiag

**Ad-free internet speed testing and network diagnostics.**

NetDiag is a privacy-focused Go web application with a lightweight browser UI. This repository contains a **working scaffold**, not a finished, independently calibrated speed test service.

## Included in this scaffold

- Go standard-library HTTP server with embedded static assets.
- HTTP idle latency, jitter, and download/upload-loaded latency (P50, P95 and added RTT; not ICMP).
- Single-stream and 4/8/16 concurrent-stream download and upload tests with a common timed measurement window.
- Server-confirmed upload byte accounting for completed requests; aborted or incomplete uploads are excluded from final results.
- Live bandwidth display, cancellation, bounded transfer sizes, a 64-transfer global limit and a 20-transfer per-peer limit.
- Server-observed IP address, automatically detected and hot-reloadable local IPinfo/MaxMind ASN lookups, and Linux TCP_INFO diagnostics.
- Optional independent IPv4/IPv6 connectivity probes with separate observed addresses and HTTP RTT samples.
- Exact-origin CORS allowlisting and a CSP that permits only configured first-party probe origins.
- Optional direct TLS termination in Go for accurate direct-peer measurements.
- Ten **public STUN endpoints** probed with WebRTC, with graceful degradation when WebRTC is unavailable.
- A homepage that hides unavailable ASN metadata, offers an accessible source tooltip next to available ASNs, and shows explicit unconfigured DNS resolver discovery.
- Docker image, Docker Compose, tests, and push-to-main GitHub Actions CI.
- No advertising, analytics trackers, user accounts, or application cookies in this scaffold.

## Run locally

Prerequisites: Go 1.27 or newer. The project selects Go 1.27.3 as its current toolchain (the latest stable patch release as of October 27, 2026). Go's automatic toolchain selection may download that version if an older supported toolchain is installed.

Go version policy: `go.mod` declares Go 1.27 as the minimum language/toolchain requirement, and its `toolchain` directive selects the current stable release used by CI and Docker. Update the selected toolchain and Docker image when a newer stable Go release is adopted.

```sh
go run ./cmd/netdiag
```

Open http://localhost:8080.

Run tests:

```sh
go test ./...
go vet ./...
```

Run with Docker:

```sh
docker compose up --build
```

## Local IPinfo and MaxMind ASN databases

At startup, NetDiag automatically detects supported **IPinfo ASN, IPinfo Lite and MaxMind GeoLite2 ASN** MMDB files from its local database directory. Use `NETDIAG_ASN_DB_DIR` to select the directory (default `./data`), or `NETDIAG_ASN_DB_PATH` to override discovery with one explicit file. The MMDB type is validated before use. When the server runs, a background watcher checks for valid atomic file replacements (default every 5 seconds) without waiting for an HTTP request. Failed replacements retain the last verified snapshot and expose a stale database status. Set `NETDIAG_ASN_RELOAD_INTERVAL=10s` to adjust the interval (1 second–1 hour). Queries are local and no credentials or databases are bundled. See [ASN setup, priority order, updates and licensing](docs/asn.md).

## API

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/health` | Health status |
| GET | `/api/config` | Browser test defaults |
| GET | `/api/ping` | HTTP 204 latency probe |
| GET | `/api/download?bytes=4194304` | Uncompressed, streamed test bytes (max 16 MiB/request) |
| POST | `/api/upload` | Receive and discard request body (max 16 MiB/request) |
| GET | `/api/ip` | Direct peer IP, address family, and locally detected IPinfo or MaxMind ASN metadata |
| GET | `/api/diag/tcp` | Server TCP_INFO on Linux, if an accepted TCP socket is accessible |
| GET | `/api/diag/address?family=4` | Require an actual IPv4 peer (409 on family mismatch) |
| GET | `/api/diag/address?family=6` | Require an actual IPv6 peer (409 on family mismatch) |

## Independent IPv4 / IPv6 probes

By default, the homepage shows both independent address-family checks as **Not configured**. It does not assume that an address observed on the main HTTP connection proves dual-stack support.

For a public HTTPS deployment, configure the main site's environment:

```sh
NETDIAG_IPV4_ORIGIN=https://ipv4.example.test
NETDIAG_IPV6_ORIGIN=https://ipv6.example.test
```

Run a NetDiag instance behind each probe hostname. Publish **A-only** DNS for the IPv4 hostname and **AAAA-only** DNS for the IPv6 hostname. Configure each probe server with the exact website origin permitted to read the JSON response:

```sh
NETDIAG_PROBE_ALLOWED_ORIGINS=https://netdiag.example.test
```

For direct TLS termination by Go (which preserves the actual peer address and makes Linux TCP_INFO relevant to the client-facing connection), supply both certificate files:

```sh
NETDIAG_TLS_CERT_FILE=/etc/netdiag/fullchain.pem
NETDIAG_TLS_KEY_FILE=/etc/netdiag/privkey.pem
PORT=443
```

The certificate must cover each public probe hostname. Mount certificates securely; never commit private keys. Plain HTTP origins are allowed only on loopback for development. The application validates configured origins and does not permit wildcard CORS.

A reverse proxy that terminates the client connection **changes the meaning of the result**: the measured peer is the reverse proxy, not the browser. NetDiag deliberately ignores arbitrary `Forwarded` and `X-Forwarded-For` headers. See [deployment guidance](docs/deployment.md).

## Measurement and deployment limitations

- **Concurrency means concurrent HTTP streams/requests**, not necessarily distinct TCP connections. HTTP/2 may multiplex them.
- Upload live speed uses browser-side XHR progress events, but **final upload results include only complete requests acknowledged by the Go server**. The result can understate throughput on slow or interrupted links because incomplete in-flight uploads are intentionally excluded.
- Download speed uses bytes actually read by the browser and measured elapsed time. Loaded latency is HTTP RTT sampled while each transfer stage runs, compared with the idle RTT median. Sampling and throughput results have not been calibrated against reference measurement tools.
- `TCP_INFO` measures the **Go server's socket**. If a reverse proxy or load balancer terminates the browser connection, the results describe the upstream socket, **not** the browser-to-edge path. `totalRetransmissions` is a lifetime counter, not a per-test retransmission delta.
- `/api/ip` reports the directly connected HTTP peer. Do not trust arbitrary `X-Forwarded-For` headers; configure a trusted-proxy policy before enabling client IP reporting behind a reverse proxy.
- ASN lookup requires an operator-provided, independently updated IPinfo or MaxMind MMDB file. DNS resolver discovery still requires an authoritative DNS observer and is **not implemented**.
- STUN only measures ICE server-reflexive candidates and does **not** reliably determine a NAT's cone/symmetric classification. Browser restrictions and blocked UDP cause inconclusive results.
- Automatic STUN probes are stopped when a bandwidth test starts to reduce interference. The public STUN server list is an **unverified integration candidate list**, not a promise of uptime, commercial-use permission, or provider support. Confirm each provider's terms before commercial deployment.
- Disable or replace third-party STUN probes if they do not meet your privacy and contractual requirements. These probes automatically contact external services on page load.
- This scaffold is not production hardened. Add per-IP quotas, trusted-proxy configuration, measurement-session accounting, monitoring, traffic budgets, real TLS termination, and legal review before a public launch.

## Project structure

```text
cmd/netdiag/              Application entry point
internal/server/          API handlers, limits and tests
internal/diagnostics/     Linux TCP_INFO implementation
web/                      Embedded HTML, CSS and JavaScript
docs/                     Architecture, deployment and roadmap
.github/workflows/        Go CI (push to main)
Dockerfile                Multi-stage container build
compose.yaml              Local container development
```

## Privacy

NetDiag intentionally has no advertising or analytics integration. However, HTTP connections expose an IP address to the server, and enabled STUN requests expose connection metadata to third parties. The website includes a [Privacy Policy](web/assets/privacy.html) page (served at `/privacy.html`) describing the default software behavior. The operator must add deployment-specific contact, logging, retention, and other privacy disclosures before public launch. See `docs/deployment.md`.

## License

This repository retains its original **GNU Affero General Public License v3.0 (AGPL-3.0)**. See [LICENSE](LICENSE).
