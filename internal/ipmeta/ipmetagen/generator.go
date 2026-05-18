package ipmetagen

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"go/format"
	"io"
	"math/big"
	"net/netip"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// AddressFamily identifies how decimal IP range bounds should be converted.
type AddressFamily string

const (
	IPv4 AddressFamily = "ipv4"
	IPv6 AddressFamily = "ipv6"
)

// CSVInput describes one IP2Location LITE CSV input.
type CSVInput struct {
	Name   string
	Family AddressFamily
	Reader io.Reader
}

// GenerateOptions configures generated ipmeta data source output.
type GenerateOptions struct {
	PackageName     string
	CountryOnly     bool
	CountryAssetDir string
	CountryCSVs     []CSVInput
	CityCSVs        []CSVInput
	ASNCSVs         []CSVInput
}

type geoRecord struct {
	first       netip.Addr
	last        netip.Addr
	countryCode string
	region      string
	city        string
}

type networkRecord struct {
	first    netip.Addr
	last     netip.Addr
	provider string
	asn      int
	asnOrg   string
}

// Generate writes a gofmt-formatted ipmeta data file from IP2Location LITE CSVs.
func Generate(w io.Writer, opts GenerateOptions) error {
	packageName := strings.TrimSpace(opts.PackageName)
	if packageName == "" {
		packageName = "ipmeta"
	}

	countryRecords, err := readGeoRecords(opts.CountryCSVs)
	if err != nil {
		return err
	}
	if opts.CountryOnly {
		if opts.CountryAssetDir != "" {
			if err := writePackedCountryAssets(opts.CountryAssetDir, countryRecords); err != nil {
				return err
			}
			source, err := renderCountryOnlyEmbedded(packageName)
			if err != nil {
				return err
			}
			_, err = w.Write(source)
			return err
		}
		source, err := renderCountryOnly(packageName, countryRecords)
		if err != nil {
			return err
		}
		_, err = w.Write(source)
		return err
	}
	cityRecords, err := readGeoRecords(opts.CityCSVs)
	if err != nil {
		return err
	}
	networkRecords, err := readNetworkRecords(opts.ASNCSVs)
	if err != nil {
		return err
	}

	source, err := render(packageName, countryRecords, cityRecords, networkRecords)
	if err != nil {
		return err
	}
	_, err = w.Write(source)
	return err
}

func readGeoRecords(inputs []CSVInput) ([]geoRecord, error) {
	records := []geoRecord{}
	for _, input := range inputs {
		next, err := readGeoCSV(input)
		if err != nil {
			return nil, err
		}
		records = append(records, next...)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].first.Compare(records[j].first) < 0 })
	return records, nil
}

func readGeoCSV(input CSVInput) ([]geoRecord, error) {
	rows, err := readCSV(input, parseGeoRow)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func readNetworkRecords(inputs []CSVInput) ([]networkRecord, error) {
	records := []networkRecord{}
	for _, input := range inputs {
		next, err := readCSV(input, parseNetworkRow)
		if err != nil {
			return nil, err
		}
		records = append(records, next...)
	}
	sort.Slice(records, func(i, j int) bool { return records[i].first.Compare(records[j].first) < 0 })
	return records, nil
}

func readCSV[T any](input CSVInput, parse func(CSVInput, []string) (T, error)) ([]T, error) {
	reader := csv.NewReader(input.Reader)
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true

	records := []T{}
	line := 0
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", input.Name, line, err)
		}
		if len(row) == 0 || isHeaderRow(row) {
			continue
		}
		record, err := parse(input, row)
		if err != nil {
			return nil, fmt.Errorf("%s line %d: %w", input.Name, line, err)
		}
		records = append(records, record)
	}
	return records, nil
}

func parseGeoRow(input CSVInput, row []string) (geoRecord, error) {
	if len(row) != 4 && len(row) < 6 {
		return geoRecord{}, fmt.Errorf("expected DB1 or DB3 row, got %d fields", len(row))
	}
	first, last, err := parseRange(input.Family, row[0], row[1])
	if err != nil {
		return geoRecord{}, err
	}
	record := geoRecord{
		first:       first,
		last:        last,
		countryCode: cleanCountryCode(row[2]),
	}
	if len(row) >= 6 {
		record.region = cleanField(row[4])
		record.city = cleanField(row[5])
	}
	return record, nil
}

func parseNetworkRow(input CSVInput, row []string) (networkRecord, error) {
	if len(row) < 5 {
		return networkRecord{}, fmt.Errorf("expected ASN row, got %d fields", len(row))
	}
	first, last, err := parseRange(input.Family, row[0], row[1])
	if err != nil {
		return networkRecord{}, err
	}
	asn := 0
	if value := cleanField(row[3]); value != "" {
		asn, err = strconv.Atoi(value)
		if err != nil {
			return networkRecord{}, fmt.Errorf("parse ASN %q: %w", row[3], err)
		}
	}
	asnOrg := cleanField(row[4])
	return networkRecord{first: first, last: last, provider: asnOrg, asn: asn, asnOrg: asnOrg}, nil
}

func parseRange(family AddressFamily, firstRaw string, lastRaw string) (netip.Addr, netip.Addr, error) {
	first, err := decimalToAddr(family, firstRaw)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	last, err := decimalToAddr(family, lastRaw)
	if err != nil {
		return netip.Addr{}, netip.Addr{}, err
	}
	return first, last, nil
}

func decimalToAddr(family AddressFamily, raw string) (netip.Addr, error) {
	value, ok := new(big.Int).SetString(strings.TrimSpace(raw), 10)
	if !ok || value.Sign() < 0 {
		return netip.Addr{}, fmt.Errorf("parse IP integer %q", raw)
	}
	switch family {
	case IPv4:
		if !value.IsUint64() || value.Uint64() > 0xffffffff {
			return netip.Addr{}, fmt.Errorf("IPv4 integer out of range %q", raw)
		}
		var addr [4]byte
		value.FillBytes(addr[:])
		return netip.AddrFrom4(addr), nil
	case IPv6:
		bytes := value.Bytes()
		if len(bytes) > 16 {
			return netip.Addr{}, fmt.Errorf("IPv6 integer out of range %q", raw)
		}
		var addr [16]byte
		copy(addr[16-len(bytes):], bytes)
		return netip.AddrFrom16(addr), nil
	default:
		return netip.Addr{}, fmt.Errorf("unsupported address family %q", family)
	}
}

func cleanField(value string) string {
	value = strings.TrimSpace(value)
	if value == "-" {
		return ""
	}
	return value
}

func cleanCountryCode(value string) string {
	value = strings.TrimSpace(value)
	if value == "-" {
		return "ZZ"
	}
	return value
}

func isHeaderRow(row []string) bool {
	return len(row) > 0 && strings.EqualFold(strings.TrimSpace(row[0]), "ip_from")
}

func render(packageName string, countryRecords []geoRecord, cityRecords []geoRecord, networkRecords []networkRecord) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by go run ./cmd/ipmeta-generate; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "//\n")
	fmt.Fprintf(&b, "// HitKeep uses the IP2Location LITE database for IP geolocation.\n")
	fmt.Fprintf(&b, "// https://www.ip2location.com\n")
	fmt.Fprintf(&b, "package %s\n\n", packageName)
	fmt.Fprintf(&b, "import \"net/netip\"\n\n")
	writePackedCountryData(&b, countryRecords)
	fmt.Fprintf(&b, "\n")
	writeGeoRanges(&b, "embeddedCityRanges", cityRecords)
	fmt.Fprintf(&b, "\n")
	writeNetworkRanges(&b, networkRecords)
	return format.Source([]byte(b.String()))
}

func renderCountryOnlyEmbedded(packageName string) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by go run ./cmd/ipmeta-generate; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "//\n")
	fmt.Fprintf(&b, "// HitKeep uses the IP2Location LITE database for IP geolocation.\n")
	fmt.Fprintf(&b, "// https://www.ip2location.com\n")
	fmt.Fprintf(&b, "package %s\n\n", packageName)
	fmt.Fprintf(&b, "import _ \"embed\"\n\n")
	fmt.Fprintf(&b, "//go:embed %s\n", countryAssetFile)
	fmt.Fprintf(&b, "var embeddedCountryZSTDData []byte\n")
	return format.Source([]byte(b.String()))
}

func renderCountryOnly(packageName string, countryRecords []geoRecord) ([]byte, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by go run ./cmd/ipmeta-generate; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "//\n")
	fmt.Fprintf(&b, "// HitKeep uses the IP2Location LITE database for IP geolocation.\n")
	fmt.Fprintf(&b, "// https://www.ip2location.com\n")
	fmt.Fprintf(&b, "package %s\n\n", packageName)
	writePackedCountryData(&b, countryRecords)
	return format.Source([]byte(b.String()))
}

const (
	countryAssetFile = "data_country.hk.zst"
)

type packedCountryAssets struct {
	ipv4StartData []byte
	ipv4Codes     string
	ipv6StartData []byte
	ipv6Codes     string
}

func writePackedCountryAssets(dir string, records []geoRecord) error {
	assets := buildPackedCountryAssets(records)
	var raw bytes.Buffer
	raw.WriteString("HKCO")
	ipv4Count, err := checkedRangeCount(assets.ipv4StartData, 4, "country IPv4 ranges")
	if err != nil {
		return err
	}
	ipv6Count, err := checkedRangeCount(assets.ipv6StartData, 16, "country IPv6 ranges")
	if err != nil {
		return err
	}
	writeUint32(&raw, ipv4Count)
	writeUint32(&raw, ipv6Count)
	raw.Write(assets.ipv4StartData)
	raw.WriteString(assets.ipv4Codes)
	raw.Write(assets.ipv6StartData)
	raw.WriteString(assets.ipv6Codes)
	compressed, err := zstdBytes(raw.Bytes())
	if err != nil {
		return fmt.Errorf("compress country asset: %w", err)
	}
	if err := writePublicGeneratedFile(filepath.Join(dir, countryAssetFile), compressed); err != nil {
		return fmt.Errorf("write country asset %s: %w", countryAssetFile, err)
	}
	return nil
}

func buildPackedCountryAssets(records []geoRecord) packedCountryAssets {
	var assets packedCountryAssets
	var ipv4Codes strings.Builder
	var ipv6Codes strings.Builder
	for _, record := range records {
		if record.first.Is4() {
			raw := record.first.As4()
			assets.ipv4StartData = append(assets.ipv4StartData, raw[:]...)
			ipv4Codes.WriteString(normalizeGeneratedCountryCode(record.countryCode))
			continue
		}
		raw := record.first.As16()
		assets.ipv6StartData = append(assets.ipv6StartData, raw[:]...)
		ipv6Codes.WriteString(normalizeGeneratedCountryCode(record.countryCode))
	}
	assets.ipv4Codes = ipv4Codes.String()
	assets.ipv6Codes = ipv6Codes.String()
	return assets
}

func writePackedCountryData(b *strings.Builder, records []geoRecord) {
	assets := buildPackedCountryAssets(records)
	writeStringLiteral(b, "embeddedCountryIPv4StartData", string(assets.ipv4StartData))
	fmt.Fprintf(b, "\n\n")
	writeStringLiteral(b, "embeddedCountryIPv4Codes", assets.ipv4Codes)
	fmt.Fprintf(b, "\n\n")

	writeStringLiteral(b, "embeddedCountryIPv6StartData", string(assets.ipv6StartData))
	fmt.Fprintf(b, "\n\n")
	writeStringLiteral(b, "embeddedCountryIPv6Codes", assets.ipv6Codes)
}

func writeStringLiteral(b *strings.Builder, name string, value string) {
	if value == "" {
		fmt.Fprintf(b, "var %s = \"\"\n", name)
		return
	}
	fmt.Fprintf(b, "var %s = \"\" +\n", name)
	for len(value) > 0 {
		chunkLen := 160
		if len(value) < chunkLen {
			chunkLen = len(value)
		}
		fmt.Fprintf(b, "%q", value[:chunkLen])
		value = value[chunkLen:]
		if value != "" {
			fmt.Fprintf(b, " +\n")
		} else {
			fmt.Fprintf(b, "\n")
		}
	}
}

func normalizeGeneratedCountryCode(code string) string {
	if len(code) == 2 {
		return code
	}
	return "ZZ"
}

func writeGeoRanges(b *strings.Builder, name string, records []geoRecord) {
	fmt.Fprintf(b, "var %s = []geoRange{\n", name)
	for _, record := range records {
		fmt.Fprintf(b, "{first: netip.MustParseAddr(%q), last: netip.MustParseAddr(%q), metadata: Metadata{", record.first.String(), record.last.String())
		fmt.Fprintf(b, "CountryCode: %q,", record.countryCode)
		if record.region != "" {
			fmt.Fprintf(b, "Region: %q,", record.region)
		}
		if record.city != "" {
			fmt.Fprintf(b, "City: %q,", record.city)
		}
		fmt.Fprintf(b, "}},\n")
	}
	fmt.Fprintf(b, "}\n")
}

func writeNetworkRanges(b *strings.Builder, records []networkRecord) {
	fmt.Fprintf(b, "var embeddedNetworkRanges = []networkRange{\n")
	for _, record := range records {
		fmt.Fprintf(b, "{first: netip.MustParseAddr(%q), last: netip.MustParseAddr(%q), metadata: NetworkMetadata{", record.first.String(), record.last.String())
		if record.provider != "" {
			fmt.Fprintf(b, "Provider: %q,", record.provider)
		}
		if record.asn > 0 {
			fmt.Fprintf(b, "ASN: %d,", record.asn)
		}
		if record.asnOrg != "" {
			fmt.Fprintf(b, "ASNOrg: %q,", record.asnOrg)
		}
		fmt.Fprintf(b, "}},\n")
	}
	fmt.Fprintf(b, "}\n")
}
