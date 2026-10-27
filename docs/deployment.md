# Deployment and privacy checklist

## Development

```sh
go run ./cmd/netdiag
# or
docker compose up --build
```

The Compose configuration exposes an **HTTP development port**. HTTPS can terminate at a trusted edge for ordinary web serving, or directly in Go for direct-peer network diagnostics.

## Independent address-family diagnostics

The homepage independently checks IPv4 and IPv6 **only when dedicated origins are configured**. Leave these variables unset to show "Not configured" and avoid misleading dual-stack claims.

Configure the main website with:

```sh
NETDIAG_IPV4_ORIGIN=https://ipv4.example.test
NETDIAG_IPV6_ORIGIN=https://ipv6.example.test
```

Use separate A-only and AAAA-only DNS records for the respective hostnames and ensure that the service is reachable on each protocol. These origins must belong to the operator; third-party probe URLs are not part of the default setup.

On the probe instances set `NETDIAG_PROBE_ALLOWED_ORIGINS=https://netdiag.example.test` to allow only the exact website origin to read `/api/diag/address` across origins. This setting accepts a comma-separated list of explicit HTTPS origins when needed. Wildcards are not supported. Origins with paths, queries, credentials or fragments are rejected; loopback HTTP is accepted for development.

For measurements that reflect the browser's real TCP/IP peer, let Go terminate TLS on the public diagnostic service by configuring `NETDIAG_TLS_CERT_FILE`, `NETDIAG_TLS_KEY_FILE`, and `PORT`. Both TLS file paths must be set together. Certificates are managed externally and mounted read-only into the container. Using a terminating reverse proxy causes this API to observe the **proxy-to-Go** connection instead, even if the proxy supplies forwarding headers. In that case, do not label the returned address as the visitor's public IP.

Each independent probe makes four small browser requests per family when configured; the first warms the connection, and the following three provide an approximate median HTTP RTT. A mismatch, blocked request, unavailable origin or CORS failure is displayed as **Unavailable**; none of these individually proves the user's ISP lacks an address family.

## Before public commercial deployment

1. Publish an operator-specific Privacy Policy with contact details, purposes, applicable legal bases, data retention, third-party recipients, and rights information.
2. Review privacy and communications rules that apply to the deployment. The scaffold uses no advertising or tracking cookies, but WebRTC/STUN can expose network data to third parties.
3. Review acceptable-use and commercial terms for **every** public STUN server; remove unapproved endpoints. `ENABLE_PUBLIC_STUN` in `web/assets/app.js` can disable automatic third-party STUN checks.
4. Obtain licensed IPinfo or MaxMind ASN MMDB data, supply and update a local file, and retain required attribution. Never commit databases or updater credentials. See `docs/asn.md`.
5. Set HTTP request quotas, per-IP and global concurrency limits, upstream bandwidth caps, quotas and cost alerts.
6. Deploy HTTPS. Set strict trusted-proxy rules to identify the real visitor's address; never blindly accept `X-Forwarded-For`.
7. Do not report Go's TCP_INFO as browser-to-edge TCP_INFO when the edge proxies to Go. Use directly terminated Go TLS for diagnostic endpoints when direct-peer measurements are needed.
8. Verify headers, privacy notices, logs and data retention policies; use no non-essential browser storage by default.
9. Check IPv4-only, IPv6-only, dual-stack, blocked UDP, WebRTC disabled, proxy, VPN and mobile browser behavior.
10. Compare speed results with reference tools under controlled conditions and test realistic network/resource limits.

## ASN database deployment

The Compose example mounts `./data` read-only at `/var/lib/netdiag` and sets `NETDIAG_ASN_DB_DIR`. NetDiag automatically discovers supported IPinfo or MaxMind ASN MMDB filenames at startup. Download and update the files separately. Lookups remain local and reload valid atomic database replacements. Use `NETDIAG_ASN_DB_PATH` only to override detection. Check licensing, freshness, file permissions and the operator's network retention policies. See `docs/asn.md`.

## Resource limits currently implemented

- 64 concurrent transfer requests per Go process, with a 20-request cap per direct peer IP.
- Download capped at 16 MiB per request.
- Upload capped at 16 MiB per request.
- Transfer handler context times out after 30 seconds.

Per-peer concurrency limits count the direct TCP peer; deployments behind a reverse proxy need an explicit trusted-client policy to prevent all visitors sharing one limit. These are **not** substitutes for per-client quotas or global bandwidth budgets. Reverse proxies can change transfer behavior and measurements.

## Important note on TCP diagnostics

If HTTPS ends at Caddy/Nginx and Go runs behind it, the server socket TCP_INFO belongs to the **proxy-to-Go** connection. A separately terminated Go TCP/TLS diagnostic endpoint is needed to measure the public client-facing server socket. The current UI makes this distinction explicit.
