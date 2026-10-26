# NetDiag

**Ad-free internet speed testing and network diagnostics.**

NetDiag is a privacy-focused Go web application with a lightweight browser UI. This repository contains a **working scaffold**, not a finished, independently calibrated speed test service.

## Included in this scaffold

- Go standard-library HTTP server with embedded static assets.
- HTTP latency and jitter samples (browser-side; not ICMP).
- Single-stream and 4/8/16 concurrent-stream download and upload tests.
- Live bandwidth display, cancellation, bounded transfer sizes, and basic server concurrency limits.
- Server-observed IP address and Linux TCP_INFO diagnostics.
- Ten **public STUN endpoints** probed with WebRTC, with graceful degradation when WebRTC is unavailable.
- A homepage with transparent unconfigured states for ASN and DNS resolver discovery.
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

## API

| Method | Route | Purpose |
| --- | --- | --- |
| GET | `/api/health` | Health status |
| GET | `/api/config` | Browser test defaults |
| GET | `/api/ping` | HTTP 204 latency probe |
| GET | `/api/download?bytes=4194304` | Uncompressed, streamed test bytes (max 16 MiB/request) |
| POST | `/api/upload` | Receive and discard request body (max 16 MiB/request) |
| GET | `/api/ip` | Direct peer IP and address family; ASN intentionally unconfigured |
| GET | `/api/diag/tcp` | Server TCP_INFO on Linux, if an accepted TCP socket is accessible |

## Measurement and deployment limitations

- **Concurrency means concurrent HTTP streams/requests**, not necessarily distinct TCP connections. HTTP/2 may multiplex them.
- Upload speed uses browser-side XHR progress events and is an **estimate**, including potentially unacknowledged in-flight bytes.
- Download speed is based on bytes read by the browser in a fixed window. These tests have not been calibrated against reference measurement tools.
- `TCP_INFO` measures the **Go server's socket**. If a reverse proxy or load balancer terminates the browser connection, the results describe the upstream socket, **not** the browser-to-edge path. `totalRetransmissions` is a lifetime counter, not a per-test retransmission delta.
- `/api/ip` reports the directly connected HTTP peer. Do not trust arbitrary `X-Forwarded-For` headers; configure a trusted-proxy policy before enabling client IP reporting behind a reverse proxy.
- ASN and DNS resolver discovery require separately licensed IP metadata and an authoritative DNS observer, respectively; they are **not implemented** in this scaffold.
- STUN only measures ICE server-reflexive candidates and does **not** reliably determine a NAT's cone/symmetric classification. Browser restrictions and blocked UDP cause inconclusive results.
- The public STUN server list is an **unverified integration candidate list**, not a promise of uptime, commercial-use permission, or provider support. Confirm each provider's terms before commercial deployment.
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

NetDiag intentionally has no advertising or analytics integration. However, HTTP connections expose an IP address to the server, and enabled STUN requests expose connection metadata to third parties. The included `/privacy.html` is a **technical draft**, not an operator-specific GDPR notice. Consult `docs/deployment.md` before EU deployment.

## License

This repository retains its original **GNU Affero General Public License v3.0 (AGPL-3.0)**. See [LICENSE](LICENSE).
