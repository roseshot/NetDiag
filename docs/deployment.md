# Deployment and privacy checklist

## Development

```sh
go run ./cmd/netdiag
# or
docker compose up --build
```

The Compose configuration exposes an **HTTP development port**. Terminate HTTPS on a trusted edge for public deployment.

## Before public commercial deployment

1. Publish an operator-specific Privacy Policy with contact details, purposes, applicable legal bases, data retention, third-party recipients, and rights information.
2. Review privacy and communications rules that apply to the deployment. The scaffold uses no advertising or tracking cookies, but WebRTC/STUN can expose network data to third parties.
3. Review acceptable-use and commercial terms for **every** public STUN server; remove unapproved endpoints. `ENABLE_PUBLIC_STUN` in `web/assets/app.js` can disable automatic third-party STUN checks.
4. Choose and attribute an appropriately licensed local IP-to-ASN database before enabling ASN lookups.
5. Set HTTP request quotas, per-IP and global concurrency limits, upstream bandwidth caps, quotas and cost alerts.
6. Deploy HTTPS. Set strict trusted-proxy rules to identify the real visitor's address; never blindly accept `X-Forwarded-For`.
7. Do not report Go's TCP_INFO as browser-to-edge TCP_INFO when the edge proxies to Go. Deploy a dedicated TCP termination endpoint if that is needed.
8. Verify headers, privacy notices, logs and data retention policies; use no non-essential browser storage by default.
9. Check IPv4-only, IPv6-only, dual-stack, blocked UDP, WebRTC disabled, proxy, VPN and mobile browser behavior.
10. Compare speed results with reference tools under controlled conditions and test realistic network/resource limits.

## Resource limits currently implemented

- 64 concurrent transfer requests per Go process.
- Download capped at 16 MiB per request.
- Upload capped at 16 MiB per request.
- Transfer handler context times out after 30 seconds.

These are **not** substitutes for per-client quotas or global bandwidth budgets. Reverse proxies can change transfer behavior and measurements.

## Important note on TCP diagnostics

If HTTPS ends at Caddy/Nginx and Go runs behind it, the server socket TCP_INFO belongs to the **proxy-to-Go** connection. A separately terminated Go TCP/TLS diagnostic endpoint is needed to measure the public client-facing server socket. The current UI makes this distinction explicit.
