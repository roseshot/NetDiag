# DNS Resolver Discovery

The DNS observer is **optional** and off by default. When configured, it runs a small authoritative DNS service for a dedicated zone and records the addresses of recursive DNS peers contacting it. These can be upstream forwarders or shared resolvers; they are **not guaranteed to be the DNS servers configured on the visitor's device**.

## Setup

1. Delegate a dedicated observation zone such as `observe.example.net` to an authoritative NS hostname such as `ns1.example.net`. Set the NS hostname's DNS address in the parent zone and any required glue.
2. Expose the NetDiag authoritative DNS listener publicly over **UDP and TCP 53**. For a host listener on a nonprivileged port, use a verified 53-to-1053 port map.
3. Point the challenge subdomains to a publicly routed IPv4 address **you control** that serves NetDiag over HTTPS, directly or through a trusted reverse proxy. NetDiag now serves the probe GIF itself at `https://<challenge>.observe.example.net/pixel.gif`, but you must still install a valid wildcard TLS certificate for `*.observe.example.net` and route these HTTPS hostnames to the Go service.
4. Set these environment variables:

```sh
NETDIAG_DNS_ZONE=observe.example.net
NETDIAG_DNS_NS=ns1.example.net
NETDIAG_DNS_LISTEN=:1053
NETDIAG_DNS_SINK_IPV4=<your-public-ipv4>
```

Without `NETDIAG_DNS_ZONE`, the feature remains disabled and no DNS listener or session is created. An incomplete/invalid configured zone makes startup fail rather than silently display false results. The application only answers its zone's NS/SOA and registered challenge queries; it is **not** a recursive resolver. For the HTTPS probe, NetDiag only responds with a fixed, cache-disabled 1×1 GIF for currently valid challenge hostnames. Expired, unknown or nested observation hostnames, and all other paths on challenge hosts, return HTTP 404 rather than serving the primary application.

## How it works

A visitor explicitly clicks **Check DNS**. The browser calls `POST /api/dns/start` to get a short-lived (two-minute) session token and randomized DNS challenge hostname. The browser requests a HTTPS image from the challenge hostname; NetDiag serves a fixed transparent pixel if the request reaches its HTTPS handler. The authoritative DNS server observes any recursive DNS peer querying that name. The browser polls `GET /api/dns/result?token=<secret>` for up to ten seconds. No query observed means **Inconclusive**, not a diagnosis of unavailable DNS.

Each session has independent random result and challenge tokens. HTTP session results are restricted to the same direct HTTP peer and an unguessable token. All observations reside **only in memory**, expire after two minutes, and disappear after process shutdown. The system enforces up to 4,096 sessions total, three active sessions per direct HTTP peer, and eight unique resolver addresses per session. Cross-site session starts are rejected and CORS is not enabled. DNS requests outside the delegated observation zone are refused.

If a local MaxMind or IPinfo ASN MMDB is available, the result can include the observed resolver's AS number and organization. No remote ASN API is used.

## Important limitations

- DNS delegation, a public TCP/UDP 53 mapping, wildcard HTTPS certificates and routing of the observation hosts to the NetDiag web process remain operational prerequisites. The application does not provision DNS, TLS certificates or network routes.
- Browser security policies, encrypted DNS, resolver forwarding, DNS caching, VPNs, blocked UDP or unsuccessful sink connections can yield no observation.
- A shared resolver or intermediate forwarder may contact the authority on behalf of many visitors; observed IPs are **not** equivalent to local DNS settings.
- Delegation to an NS within the observation zone requires appropriate glue and address records (NetDiag does not automatically manage them).
- The HTTPS challenge hostname must reach NetDiag's HTTP handler (either via its direct TLS listener or an HTTPS reverse proxy). The `/pixel.gif` response is an identical, fixed 1×1 image for every active challenge; no visitor-specific content is returned. Observation-zone hosts cannot access the main UI or application APIs.
- Protect a public authoritative DNS listener with firewall, monitoring, DNS abuse budgets and appropriate deployment controls.

## API

| Method | Endpoint | Description |
| --- | --- | --- |
| POST | `/api/dns/start` | Create an opt-in challenge; 503 when disabled |
| GET | `/api/dns/result?token=...` | Return ephemeral observed resolver IPs and optional local ASN metadata |

To check the authoritative listener locally:

```sh
dig @127.0.0.1 -p 1053 observe.example.net NS
dig @127.0.0.1 -p 1053 observe.example.net SOA
```

See [deployment](deployment.md) and the [Privacy Policy](../web/assets/privacy.html).