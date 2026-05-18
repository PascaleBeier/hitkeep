# IP Metadata Data Generation

HitKeep uses embedded IP2Location LITE-derived data for coarse country, region,
city, provider, ASN, and ASN organization analytics metadata. Runtime lookups do
not download data and do not need an IP2Location token.

## Release Refresh

For the normal release refresh, set `IP2LOCATION_DOWNLOAD_TOKEN` and run the
generator without CLI arguments:

```sh
go run ./cmd/ipmeta-generate
```

The no-argument command is the release path. It:

- mirrors the public DB1 country ZIPs used by `github.com/phuslu/iploc`
- writes the compact country asset and embed source:
  `data_country.hk.zst` and `data_country_lite.go`
- downloads the IPv6 BIN packages `DB3LITEBINIPV6` and `DBASNLITEBINIPV6`
- distills those BIN payloads into HitKeep-owned compressed lookup assets:
  `data_city.hk.zst`, `data_asn.hk.zst`, and `data_city_network_lite.go`
- discards upstream ZIP/BIN payloads after generation

IP2Location's IPv6 BIN packages cover both IPv4 and IPv6 lookups, so the normal
release refresh does not download CSV/CIDR variants or separate IPv4 BIN
packages. Raw upstream BIN files, text metadata files, and separate source
dataset IPv4 artifacts are not checked in.

The generator can still accept explicit CSV/ZIP inputs for tests and unusual
fixture work, but that is not the release workflow. Run `go run
./cmd/ipmeta-generate -h` if you need those maintenance flags.

## Attribution

The generated source includes IP2Location attribution:

HitKeep uses the IP2Location LITE database for IP geolocation.
https://www.ip2location.com
