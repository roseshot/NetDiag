# Local ASN databases: IPinfo and MaxMind

NetDiag supports **IPinfo ASN** and **MaxMind GeoLite2 ASN** in the MMDB format. Queries are local: the web application never forwards visitor IP addresses to either provider.

## Automatic startup detection

When `NETDIAG_ASN_DB_PATH` is unset, NetDiag scans `NETDIAG_ASN_DB_DIR` immediately when the server starts. It defaults to `./data` for local Go development and `/var/lib/netdiag` in Docker Compose.

Supported file names, in descending priority:

1. `ipinfo_asn.mmdb` — IPinfo ASN database
2. `ipinfo_lite.mmdb` — IPinfo Lite database (ASN fields only)
3. `asn.mmdb` — legacy IPinfo free ASN database
4. `country_asn.mmdb` — legacy IPinfo free Country + ASN database
5. `GeoLite2-ASN.mmdb` — MaxMind GeoLite2 ASN database

The reader **validates the database's MMDB metadata and integrity**. A name alone is not sufficient. It selects the first *valid* ASN-capable database, skipping missing, corrupt and incompatible files. This is deterministic priority order, **not a comparison of freshness or accuracy**.

To select one database explicitly, set `NETDIAG_ASN_DB_PATH` to its full file path. The explicit file overrides discovery, and a missing or invalid override reports `unavailable` rather than silently selecting another provider.

The selected database is logged during server startup. The `/api/ip` response and homepage also report the **actual provider** and MMDB database type when a database is active. No provider is claimed when none is available.

NetDiag checks file metadata at most once per minute on lookups and safely replaces an open reader after a new MMDB has passed validation. Operators should **atomically replace files** rather than modify a memory-mapped MMDB in place. Newly added files and provider changes are detected without restarting when a lookup triggers a check.

## Install an MMDB

Place **one** downloaded MMDB at a recognized name under `./data`:

```text
data/ipinfo_asn.mmdb
# or
data/ipinfo_lite.mmdb
# or
data/GeoLite2-ASN.mmdb
```

To run with Docker Compose:

```sh
docker compose up --build
```

The volume is mounted read-only at `/var/lib/netdiag` inside the application. Only the updater should have write access. The database is **not** committed to the repository or copied into the Docker image. No database download takes place at startup.

For a standalone process:

```sh
NETDIAG_ASN_DB_DIR=/srv/netdiag/data go run ./cmd/netdiag
```

Or override the discovered file:

```sh
NETDIAG_ASN_DB_PATH=/srv/netdiag/data/ipinfo_lite.mmdb go run ./cmd/netdiag
```

## Obtain and update the files

**IPinfo Lite:** Download an MMDB using your IPinfo account and download token from the [IPinfo Lite database documentation](https://ipinfo.io/developers/ipinfo-lite-database). IPinfo's Lite download is distributed under the [Creative Commons Attribution-ShareAlike 4.0 license](https://creativecommons.org/licenses/by-sa/4.0/). Attribution must link to IPinfo. Check the license implications for any redistributed or derived data.

**IPinfo ASN:** See the [IPinfo ASN database documentation](https://ipinfo.io/developers/asn-database). Availability and permitted use depend on the operator's IPinfo account or contract.

**MaxMind GeoLite2 ASN:** Register for a MaxMind account, obtain a license key, and use their official `geoipupdate` utility with `EditionIDs GeoLite2-ASN`. Keep the updater's config and license key outside the repository. See [MaxMind's update guidance](https://dev.maxmind.com/geoip/updating-databases/).

Each database must be updated separately by the operator using the relevant vendor's documented process. **Automatic detection does not download, license, or schedule database updates.** Keep credentials out of application environment variables, browser code, logs and Git.

## API

`GET /api/ip` includes:

| Field | Meaning |
| --- | --- |
| `asn` | Numeric ASN on a match, otherwise `null` |
| `asnStatus` | `completed`, `not_configured`, `unavailable`, `not_found`, or `not_applicable` |
| `asnOrganization` | AS organization name |
| `asnDomain` | Organization domain where available (IPinfo) |
| `asnNetwork` | Matching MMDB CIDR, **not necessarily a BGP-announced prefix** |
| `asnDatabaseBuiltAt` | Database build timestamp if provided |
| `asnSource` | `IPinfo` or `MaxMind GeoLite2 ASN`, empty when none is loaded |
| `asnDatabaseType` | Detected internal MMDB database type |

The IPinfo ASN value is stored as a string such as `AS13335`; NetDiag validates and converts it to a number before returning the API response. IPinfo Lite provides `as_name` and `as_domain` while IPinfo ASN provides `name` and `domain`. MaxMind's fields are `autonomous_system_number` and `autonomous_system_organization`.

Only the **direct HTTP peer** is looked up. Behind a terminating proxy, it can be the proxy IP. NetDiag does not trust unverified forwarding headers.

## Attribution

The site retains MaxMind attribution and also displays: **IP address data powered by IPinfo** with a link to [IPinfo](https://ipinfo.io). Operators must ensure their selected databases and use comply with the provider's applicable terms.

No full IP-to-ASN dataset or vendor credential is bundled or distributed.
