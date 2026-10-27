# MaxMind GeoLite2 ASN

NetDiag reads **MaxMind GeoLite2 ASN**, not a GeoIP City or Country database. It looks up the autonomous system number and registered AS organization for the HTTP peer, including IPv4 and IPv6 where data is available.

The reader runs locally in Go. No visitor lookup is forwarded to MaxMind. The matching MMDB range is **not necessarily the announced BGP prefix**. Network operators should not treat ASN metadata as independently verified ownership or routing information.

## Obtain the database

1. Create a MaxMind account and accept the applicable GeoLite terms.
2. Obtain an account ID and a license key.
3. Download the **GeoLite2-ASN MMDB** database, or use MaxMind's official `geoipupdate` tool.
4. Keep the file outside Git. Set `NETDIAG_ASN_DB_PATH` to its absolute path.

A local Compose deployment expects the file at `./data/GeoLite2-ASN.mmdb`. Docker mounts the directory read-only at `/var/lib/netdiag` and sets the correct environment variable. The Go application starts normally without a database, but the ASN field shows **Database unavailable**.

For a standalone process:

```sh
NETDIAG_ASN_DB_PATH=/var/lib/netdiag/GeoLite2-ASN.mmdb go run ./cmd/netdiag
```

## Automated updates

MaxMind provides a separate updater called `geoipupdate`. Download the operator-specific `GeoIP.conf` from the MaxMind account portal and set its `EditionIDs` value to `GeoLite2-ASN`. Keep that configuration and license key **outside the repository**. Configure the tool to write to your dedicated MMDB directory, which NetDiag mounts read-only.

Schedule the official updater regularly (for example, daily). It must replace database files atomically instead of modifying a mapped MMDB in place. NetDiag checks for a file change at most once per minute on lookup, verifies the candidate database and transparently replaces the reader. Missing, corrupt, or wrong-type databases yield an explicit unavailable status rather than synthetic ASN values.

MaxMind requires GeoLite users to keep their data up to date and remove superseded files within the period specified by their license. Check the latest terms and the database's stated update schedule before deployment. Do not redistribute the MMDB, embed it in a Docker image or commit it to Git without a separate license that permits redistribution.

## Response fields

`GET /api/ip` retains the existing fields and adds:

| Field | Meaning |
| --- | --- |
| `asnStatus` | `completed`, `not_configured`, `unavailable`, `not_found`, or `not_applicable` |
| `asn` | AS number if an entry matches, otherwise `null` |
| `asnOrganization` | Organization string from GeoLite2 ASN |
| `asnNetwork` | MMDB matching IP range, **not** a BGP announced prefix |
| `asnDatabaseBuiltAt` | Database build timestamp where provided |
| `asnSource` | `MaxMind GeoLite2 ASN` |

Lookups use the **direct HTTP peer** and deliberately ignore arbitrary forwarding headers. If HTTPS ends at an upstream proxy, ASN data may describe the proxy rather than the visitor.

## Attribution and licensing

This product includes GeoLite Data created by MaxMind, available from [MaxMind](https://www.maxmind.com).

MaxMind's GeoLite terms permit publicly displayed data with required attribution in many common uses, but the operator remains responsible for checking that its actual commercial service, display, update, and redistribution arrangements are permitted. The project does **not** redistribute a database.

Official documentation:

- [GeoLite databases](https://dev.maxmind.com/geoip/geolite2-free-geolocation-data/)
- [ASN database format and fields](https://dev.maxmind.com/geoip/docs/databases/asn/)
- [Updating GeoIP and GeoLite databases](https://dev.maxmind.com/geoip/updating-databases/)
