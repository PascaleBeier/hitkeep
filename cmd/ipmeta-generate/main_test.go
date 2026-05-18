package main

import (
	"archive/zip"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hitkeep/internal/ipmeta/ipmetagen"
)

func TestRunGeneratesIPMetaDataFromDB1DB3AndASNInputs(t *testing.T) {
	dir := t.TempDir()
	db1Path := filepath.Join(dir, "IP2LOCATION-LITE-DB1.CSV")
	db3Path := filepath.Join(dir, "IP2LOCATION-LITE-DB3.CSV")
	asnPath := filepath.Join(dir, "IP2LOCATION-LITE-ASN.CSV")
	outPath := filepath.Join(dir, "data_lite.go")

	if err := os.WriteFile(db1Path, []byte(strings.Join([]string{
		`"ip_from","ip_to","country_code","country_name"`,
		`"134744064","134744319","US","United States of America"`,
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write db1 fixture: %v", err)
	}
	if err := os.WriteFile(db3Path, []byte(strings.Join([]string{
		`"ip_from","ip_to","country_code","country_name","region_name","city_name"`,
		`"134744064","134744319","US","United States of America","California","Mountain View"`,
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write db3 fixture: %v", err)
	}
	if err := os.WriteFile(asnPath, []byte(strings.Join([]string{
		`"ip_from","ip_to","cidr","asn","as"`,
		`"134744064","134744319","8.8.8.0/24","15169","Google LLC"`,
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write asn fixture: %v", err)
	}

	if err := run([]string{"-db1", db1Path, "-db3", db3Path, "-asn", asnPath, "-out", outPath}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}

	generated, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated output: %v", err)
	}
	for _, want := range []string{"embeddedCountryIPv4StartData", "embeddedCountryIPv4Codes", "embeddedCityRanges", "Mountain View", "Google LLC", "15169"} {
		if !strings.Contains(string(generated), want) {
			t.Fatalf("expected generated output to contain %q:\n%s", want, string(generated))
		}
	}
}

func TestRunAcceptsOfficialLiteZipInputs(t *testing.T) {
	dir := t.TempDir()
	db1Path := filepath.Join(dir, "IP2LOCATION-LITE-DB1.CSV.ZIP")
	outPath := filepath.Join(dir, "data_lite.go")
	if err := writeZipFixture(db1Path, "IP2LOCATION-LITE-DB1.CSV", strings.Join([]string{
		`"ip_from","ip_to","country_code","country_name"`,
		`"134744064","134744319","US","United States of America"`,
	}, "\n")); err != nil {
		t.Fatalf("write zip fixture: %v", err)
	}

	if err := run([]string{"-db1", db1Path, "-out", outPath}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}

	generated, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated output: %v", err)
	}
	if !strings.Contains(string(generated), "embeddedCountryIPv4StartData") || !strings.Contains(string(generated), `"US"`) {
		t.Fatalf("expected generated output to include DB1 country data:\n%s", string(generated))
	}
}

func TestRunSupportsIplocCountryDownloadMode(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "data_country_lite.go")
	server := newIP2LocationFixtureServer(t)
	originalCountryURLs := iplocCountryURLs
	iplocCountryURLs = []countryDownload{
		{source: server.URL + "/IP2LOCATION-LITE-DB1.CSV.ZIP", family: ipmetagen.IPv4},
		{source: server.URL + "/IP2LOCATION-LITE-DB1.IPV6.CSV.ZIP", family: ipmetagen.IPv6},
	}
	t.Cleanup(func() {
		iplocCountryURLs = originalCountryURLs
	})

	if err := run([]string{"-iploc-country", "-country-only", "-out", outPath}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}

	generated, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated output: %v", err)
	}
	for _, want := range []string{"//go:embed data_country.hk.zst", "var embeddedCountryZSTDData []byte"} {
		if !strings.Contains(string(generated), want) {
			t.Fatalf("expected generated output to contain %q:\n%s", want, string(generated))
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "data_country.hk.zst")); err != nil {
		t.Fatalf("expected country asset: %v", err)
	}
	for _, stale := range []string{"data_country_ipv4_starts.bin", "data_country_ipv4_codes.txt", "data_country_ipv6_starts.bin", "data_country_ipv6_codes.txt"} {
		if _, err := os.Stat(filepath.Join(dir, stale)); !os.IsNotExist(err) {
			t.Fatalf("expected stale country asset %s to be absent, got err=%v", stale, err)
		}
	}
}

func TestRunSupportsTokenBasedCityAndASNDownloads(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "data_lite.go")
	server := newIP2LocationTokenFixtureServer(t, "release-token")
	originalDownloadBaseURL := ip2locationDownloadBaseURL
	ip2locationDownloadBaseURL = server.URL + "/download"
	t.Cleanup(func() {
		ip2locationDownloadBaseURL = originalDownloadBaseURL
	})

	if err := run([]string{"-ip2location-lite", "-ip2location-token", "release-token", "-out", outPath}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}

	generated, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated output: %v", err)
	}
	for _, want := range []string{"Mountain View", "Google LLC", "15169"} {
		if !strings.Contains(string(generated), want) {
			t.Fatalf("expected generated output to contain %q:\n%s", want, string(generated))
		}
	}
}

func TestRunDefaultsToCompactReleaseRefresh(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "data_country_lite.go")
	countryServer := newIP2LocationFixtureServer(t)
	tokenServer := newIP2LocationReleaseBinFixtureServer(t, "release-token")
	originalCountryURLs := iplocCountryURLs
	originalDownloadBaseURL := ip2locationDownloadBaseURL
	iplocCountryURLs = []countryDownload{
		{source: countryServer.URL + "/IP2LOCATION-LITE-DB1.CSV.ZIP", family: ipmetagen.IPv4},
		{source: countryServer.URL + "/IP2LOCATION-LITE-DB1.IPV6.CSV.ZIP", family: ipmetagen.IPv6},
	}
	ip2locationDownloadBaseURL = tokenServer.URL + "/download"
	t.Setenv("IP2LOCATION_DOWNLOAD_TOKEN", "release-token")
	t.Cleanup(func() {
		iplocCountryURLs = originalCountryURLs
		ip2locationDownloadBaseURL = originalDownloadBaseURL
	})

	if err := run([]string{"-out", outPath}, io.Discard); err != nil {
		t.Fatalf("run: %v", err)
	}

	generated, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated output: %v", err)
	}
	for _, want := range []string{"//go:embed data_country.hk.zst", "var embeddedCountryZSTDData []byte"} {
		if !strings.Contains(string(generated), want) {
			t.Fatalf("expected default release output to contain %q:\n%s", want, string(generated))
		}
	}
	for _, wantFile := range []string{"data_country.hk.zst", "data_city.hk.zst", "data_asn.hk.zst"} {
		if _, err := os.Stat(filepath.Join(dir, wantFile)); err != nil {
			t.Fatalf("expected compact asset %s: %v", wantFile, err)
		}
	}
	cityNetworkSource, err := os.ReadFile(filepath.Join(dir, "data_city_network_lite.go"))
	if err != nil {
		t.Fatalf("expected city/network embed source: %v", err)
	}
	for _, want := range []string{
		"Code generated by go run ./cmd/ipmeta-generate; DO NOT EDIT.",
		"HitKeep uses the IP2Location LITE database for IP geolocation.",
		"https://www.ip2location.com",
		"//go:embed data_city.hk.zst",
		"//go:embed data_asn.hk.zst",
	} {
		if !strings.Contains(string(cityNetworkSource), want) {
			t.Fatalf("expected city/network embed source to contain %q:\n%s", want, string(cityNetworkSource))
		}
	}
	for _, stale := range []string{"data_city_lite_000.bin", "data_city_ipv4_starts.bin.gz", "data_city_meta.txt.gz", "data_asn_ipv6_starts.bin.gz", "data_asn_meta.txt.gz"} {
		if _, err := os.Stat(filepath.Join(dir, stale)); !os.IsNotExist(err) {
			t.Fatalf("expected stale asset %s to be absent, got err=%v", stale, err)
		}
	}
}

func TestRunRequiresTokenForTokenBasedDownloads(t *testing.T) {
	dir := t.TempDir()
	err := run([]string{"-ip2location-lite", "-out", filepath.Join(dir, "data_lite.go")}, io.Discard)
	if err == nil {
		t.Fatal("expected missing token to fail")
	}
	if !strings.Contains(err.Error(), "IP2LOCATION_DOWNLOAD_TOKEN") {
		t.Fatalf("expected token guidance, got %v", err)
	}
}

func TestDownloadBINDataReportsIP2LocationTextError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=UTF-8")
		_, _ = w.Write([]byte("THIS FILE CAN ONLY BE DOWNLOADED 5 TIMES WITHIN 24 HOURS"))
	}))
	defer server.Close()

	_, err := downloadBINData(server.URL + "/download?token=secret&file=DB3LITEBINIPV6")
	if err == nil {
		t.Fatal("expected invalid BIN download to fail")
	}
	for _, want := range []string{"did not return a ZIP or BIN payload", "THIS FILE CAN ONLY BE DOWNLOADED", "token=REDACTED"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("expected token to be redacted, got %v", err)
	}
}

func writeZipFixture(path string, name string, body string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := zip.NewWriter(file)
	entry, err := writer.Create(name)
	if err != nil {
		_ = writer.Close()
		return err
	}
	if _, err := entry.Write([]byte(body)); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}

func newIP2LocationTokenFixtureServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != token {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("file") {
		case "DB3LITECSV":
			writeZipResponse(t, w, "IP2LOCATION-LITE-DB3.CSV", strings.Join([]string{
				`"ip_from","ip_to","country_code","country_name","region_name","city_name"`,
				`"134744064","134744319","US","United States of America","California","Mountain View"`,
			}, "\n"))
		case "DB3LITECSVIPV6":
			writeZipResponse(t, w, "IP2LOCATION-LITE-DB3.IPV6.CSV", strings.Join([]string{
				`"ip_from","ip_to","country_code","country_name","region_name","city_name"`,
				`"42540766411282592856903984951653826560","42540766411282592856903984951653826815","NL","Netherlands","Noord-Holland","Amsterdam"`,
			}, "\n"))
		case "DBASNLITE":
			writeZipResponse(t, w, "IP2LOCATION-LITE-ASN.CSV", strings.Join([]string{
				`"ip_from","ip_to","cidr","asn","as"`,
				`"134744064","134744319","8.8.8.0/24","15169","Google LLC"`,
			}, "\n"))
		case "DBASNLITEIPV6":
			writeZipResponse(t, w, "IP2LOCATION-LITE-ASN.IPV6.CSV", strings.Join([]string{
				`"ip_from","ip_to","cidr","asn","as"`,
				`"42540766411282592856903984951653826560","42540766411282592856903984951653826815","2001:db8::/120","65536","Example Networks"`,
			}, "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
}

func newIP2LocationReleaseBinFixtureServer(t *testing.T, token string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != token {
			http.Error(w, "bad token", http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("file") {
		case "DB3LITEBINIPV6":
			writeZipBytesResponse(t, w, "IP2LOCATION-LITE-DB3.IPV6.BIN", buildCityBINFixture())
		case "DBASNLITEBINIPV6":
			writeZipBytesResponse(t, w, "IP2LOCATION-LITE-ASN.IPV6.BIN", buildASNBINFixture())
		default:
			http.NotFound(w, r)
		}
	}))
}

func newIP2LocationFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/IP2LOCATION-LITE-DB1.CSV.ZIP":
			writeZipResponse(t, w, "IP2LOCATION-LITE-DB1.CSV", strings.Join([]string{
				`"ip_from","ip_to","country_code","country_name"`,
				`"134744064","134744319","US","United States of America"`,
			}, "\n"))
		case "/IP2LOCATION-LITE-DB1.IPV6.CSV.ZIP":
			writeZipResponse(t, w, "IP2LOCATION-LITE-DB1.IPV6.CSV", strings.Join([]string{
				`"ip_from","ip_to","country_code","country_name"`,
				`"42540766411282592856903984951653826560","42540766411282592856903984951653826815","NL","Netherlands"`,
			}, "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
}

func writeZipResponse(t *testing.T, w http.ResponseWriter, name string, body string) {
	t.Helper()
	writeZipBytesResponse(t, w, name, []byte(body))
}

func writeZipBytesResponse(t *testing.T, w http.ResponseWriter, name string, body []byte) {
	t.Helper()
	w.Header().Set("Content-Type", "application/zip")
	writer := zip.NewWriter(w)
	entry, err := writer.Create(name)
	if err != nil {
		t.Fatalf("create zip response entry: %v", err)
	}
	if _, err := entry.Write(body); err != nil {
		t.Fatalf("write zip response entry: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close zip response: %v", err)
	}
}

func buildCityBINFixture() []byte {
	return buildBINFixture(binFixtureOptions{
		databaseType: 3,
		columns:      4,
		v4Row: func(row []byte, strings map[string]uint32) {
			binary.LittleEndian.PutUint32(row[0:4], 134744064)
			binary.LittleEndian.PutUint32(row[4:8], strings["US"])
			binary.LittleEndian.PutUint32(row[8:12], strings["California"])
			binary.LittleEndian.PutUint32(row[12:16], strings["Mountain View"])
		},
		v6Row: func(row []byte, strings map[string]uint32) {
			writeIPv6BINStart(row[0:16], "2001:db8::")
			binary.LittleEndian.PutUint32(row[16:20], strings["NL"])
			binary.LittleEndian.PutUint32(row[20:24], strings["Noord-Holland"])
			binary.LittleEndian.PutUint32(row[24:28], strings["Amsterdam"])
		},
		strings: []string{"US", "California", "Mountain View", "NL", "Noord-Holland", "Amsterdam"},
	})
}

func buildASNBINFixture() []byte {
	return buildBINFixture(binFixtureOptions{
		databaseType: 26,
		columns:      28,
		v4Row: func(row []byte, strings map[string]uint32) {
			binary.LittleEndian.PutUint32(row[0:4], 134744064)
			binary.LittleEndian.PutUint32(row[92:96], strings["15169"])
			binary.LittleEndian.PutUint32(row[96:100], strings["Google LLC"])
		},
		v6Row: func(row []byte, strings map[string]uint32) {
			writeIPv6BINStart(row[0:16], "2001:db8::")
			binary.LittleEndian.PutUint32(row[104:108], strings["65536"])
			binary.LittleEndian.PutUint32(row[108:112], strings["Example Networks"])
		},
		strings: []string{"15169", "Google LLC", "65536", "Example Networks"},
	})
}

type binFixtureOptions struct {
	databaseType uint8
	columns      uint8
	v4Row        func([]byte, map[string]uint32)
	v6Row        func([]byte, map[string]uint32)
	strings      []string
}

func buildBINFixture(opts binFixtureOptions) []byte {
	v4Size := int(opts.columns) * 4
	v6Size := 16 + ((int(opts.columns) - 1) * 4)
	size := 64 + v4Size + v6Size
	data := make([]byte, size)
	data[0] = opts.databaseType
	data[1] = opts.columns
	data[2] = 26
	data[3] = 5
	data[4] = 15
	binary.LittleEndian.PutUint32(data[5:9], 1)
	binary.LittleEndian.PutUint32(data[9:13], 65)
	binary.LittleEndian.PutUint32(data[13:17], 1)
	binary.LittleEndian.PutUint32(data[17:21], uint32(65+v4Size))

	stringOffsets := map[string]uint32{}
	for _, value := range opts.strings {
		stringOffsets[value] = uint32(len(data))
		data = append(data, byte(len(value)))
		data = append(data, value...)
	}
	opts.v4Row(data[64:64+v4Size], stringOffsets)
	opts.v6Row(data[64+v4Size:64+v4Size+v6Size], stringOffsets)
	binary.LittleEndian.PutUint32(data[31:35], uint32(len(data)))
	return data
}

func writeIPv6BINStart(dst []byte, raw string) {
	addr := netip.MustParseAddr(raw).As16()
	for i := range addr {
		dst[i] = addr[len(addr)-1-i]
	}
}
