package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hitkeep/internal/ipmeta/ipmetagen"
)

var httpClient = &http.Client{Timeout: 2 * time.Minute}

var ip2locationDownloadBaseURL = "https://www.ip2location.com/download"

const (
	ip2locationDB3LiteBinIPv6Code = "DB3LITEBINIPV6"
	ip2locationASNLiteBinIPv6Code = "DBASNLITEBINIPV6"
	ip2locationDB3LiteCSVCode     = "DB3LITECSV"
	ip2locationDB3LiteCSVIPv6Code = "DB3LITECSVIPV6"
	ip2locationASNLiteCSVCode     = "DBASNLITE"
	ip2locationASNLiteCSVIPv6Code = "DBASNLITEIPV6"
)

type countryDownload struct {
	source string
	family ipmetagen.AddressFamily
}

var iplocCountryURLs = []countryDownload{
	{source: "https://download.ip2location.com/lite/IP2LOCATION-LITE-DB1.CSV.ZIP", family: ipmetagen.IPv4},
	{source: "https://download.ip2location.com/lite/IP2LOCATION-LITE-DB1.IPV6.CSV.ZIP", family: ipmetagen.IPv6},
}

type pathList []string

func (p *pathList) String() string {
	return fmt.Sprint([]string(*p))
}

func (p *pathList) Set(value string) error {
	if value == "" {
		return errors.New("path must not be empty")
	}
	*p = append(*p, value)
	return nil
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "ipmeta-generate: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	var (
		db1              pathList
		db3              pathList
		asn              pathList
		db1v6            pathList
		db3v6            pathList
		asnv6            pathList
		outPath          string
		pkg              string
		iplocCountry     bool
		countryOnly      bool
		ip2locationLite  bool
		ip2locationToken string
	)

	flags := flag.NewFlagSet("ipmeta-generate", flag.ContinueOnError)
	flags.SetOutput(stdout)
	flags.Var(&db1, "db1", "IP2Location LITE DB1 IPv4 country CSV/ZIP path or URL; repeatable")
	flags.Var(&db3, "db3", "IP2Location LITE DB3 IPv4 country-region-city CSV/ZIP path or URL; repeatable")
	flags.Var(&asn, "asn", "IP2Location LITE ASN IPv4 CSV/ZIP path or URL; repeatable")
	flags.Var(&db1v6, "db1-ipv6", "IP2Location LITE DB1 IPv6 country CSV/ZIP path or URL; repeatable")
	flags.Var(&db3v6, "db3-ipv6", "IP2Location LITE DB3 IPv6 country-region-city CSV/ZIP path or URL; repeatable")
	flags.Var(&asnv6, "asn-ipv6", "IP2Location LITE ASN IPv6 CSV/ZIP path or URL; repeatable")
	flags.BoolVar(&iplocCountry, "iploc-country", false, "download the public DB1 IPv4 and IPv6 ZIPs used by github.com/phuslu/iploc")
	flags.BoolVar(&ip2locationLite, "ip2location-lite", false, "download IP2Location LITE DB3 city and ASN ZIPs using a download token")
	flags.StringVar(&ip2locationToken, "ip2location-token", "", "IP2Location download token; defaults to IP2LOCATION_DOWNLOAD_TOKEN")
	flags.BoolVar(&countryOnly, "country-only", false, "write only packed DB1 country metadata")
	flags.StringVar(&outPath, "out", "internal/ipmeta/data_country_lite.go", "generated Go output path")
	flags.StringVar(&pkg, "package", "ipmeta", "generated Go package name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	defaultReleaseRefresh := !hasExplicitInputs(db1, db3, asn, db1v6, db3v6, asnv6) && !iplocCountry && !ip2locationLite && !countryOnly
	if defaultReleaseRefresh {
		return runCompactReleaseRefresh(outPath, pkg, ip2locationToken)
	}
	appendIplocCountryInputs(&db1, &db1v6, iplocCountry)
	if err := appendIP2LocationLiteInputs(&db3, &db3v6, &asn, &asnv6, ip2locationLite, ip2locationToken); err != nil {
		return err
	}

	opts, closers, err := buildOptions(pkg, db1, db3, asn, db1v6, db3v6, asnv6)
	if err != nil {
		return err
	}
	opts.CountryOnly = countryOnly
	if countryOnly {
		opts.CountryAssetDir = filepath.Dir(outPath)
	}
	defer closeAll(closers)
	if len(opts.CountryCSVs) == 0 && len(opts.CityCSVs) == 0 && len(opts.ASNCSVs) == 0 {
		return errors.New("provide at least one -db1, -db3, -asn, -db1-ipv6, -db3-ipv6, or -asn-ipv6 input")
	}

	return writeGeneratedOutput(outPath, opts)
}

func runCompactReleaseRefresh(outPath string, pkg string, explicitToken string) error {
	token := ip2locationToken(explicitToken)
	if token == "" {
		return errors.New("provide -ip2location-token or set IP2LOCATION_DOWNLOAD_TOKEN to download DB3 and ASN LITE BIN data")
	}

	var db1 pathList
	var db1v6 pathList
	appendIplocCountryInputs(&db1, &db1v6, true)
	opts, closers, err := buildOptions(pkg, db1, nil, nil, db1v6, nil, nil)
	if err != nil {
		return err
	}
	defer closeAll(closers)
	opts.CountryOnly = true
	opts.CountryAssetDir = filepath.Dir(outPath)
	if err := writeGeneratedOutput(outPath, opts); err != nil {
		return err
	}

	cityBIN, err := downloadBINData(ip2locationDownloadURL(token, ip2locationDB3LiteBinIPv6Code))
	if err != nil {
		return err
	}
	if err := ipmetagen.WritePackedCityBINAssets(filepath.Dir(outPath), cityBIN); err != nil {
		return err
	}
	asnBIN, err := downloadBINData(ip2locationDownloadURL(token, ip2locationASNLiteBinIPv6Code))
	if err != nil {
		return err
	}
	if err := ipmetagen.WritePackedASNBINAssets(filepath.Dir(outPath), asnBIN); err != nil {
		return err
	}
	if err := ipmetagen.WritePackedCityNetworkEmbedSource(filepath.Dir(outPath), pkg); err != nil {
		return err
	}
	return nil
}

func hasExplicitInputs(lists ...pathList) bool {
	for _, list := range lists {
		if len(list) > 0 {
			return true
		}
	}
	return false
}

func appendIplocCountryInputs(db1 *pathList, db1v6 *pathList, enabled bool) {
	if !enabled {
		return
	}
	for _, download := range iplocCountryURLs {
		if download.family == ipmetagen.IPv4 {
			*db1 = append(*db1, download.source)
		}
		if download.family == ipmetagen.IPv6 {
			*db1v6 = append(*db1v6, download.source)
		}
	}
}

func appendIP2LocationLiteInputs(db3 *pathList, db3v6 *pathList, asn *pathList, asnv6 *pathList, enabled bool, explicitToken string) error {
	if !enabled {
		return nil
	}
	token := ip2locationToken(explicitToken)
	if token == "" {
		return errors.New("provide -ip2location-token or set IP2LOCATION_DOWNLOAD_TOKEN to download DB3 and ASN LITE data")
	}
	*db3 = append(*db3, ip2locationDownloadURL(token, ip2locationDB3LiteCSVCode))
	*db3v6 = append(*db3v6, ip2locationDownloadURL(token, ip2locationDB3LiteCSVIPv6Code))
	*asn = append(*asn, ip2locationDownloadURL(token, ip2locationASNLiteCSVCode))
	*asnv6 = append(*asnv6, ip2locationDownloadURL(token, ip2locationASNLiteCSVIPv6Code))
	return nil
}

func ip2locationToken(explicitToken string) string {
	token := strings.TrimSpace(explicitToken)
	if token == "" {
		token = strings.TrimSpace(os.Getenv("IP2LOCATION_DOWNLOAD_TOKEN"))
	}
	return token
}

func buildOptions(pkg string, db1, db3, asn, db1v6, db3v6, asnv6 pathList) (ipmetagen.GenerateOptions, []io.Closer, error) {
	opts := ipmetagen.GenerateOptions{PackageName: pkg}
	var closers []io.Closer
	for _, item := range []struct {
		paths       pathList
		family      ipmetagen.AddressFamily
		destination string
	}{
		{paths: db1, family: ipmetagen.IPv4, destination: "country"},
		{paths: db3, family: ipmetagen.IPv4, destination: "city"},
		{paths: asn, family: ipmetagen.IPv4, destination: "asn"},
		{paths: db1v6, family: ipmetagen.IPv6, destination: "country"},
		{paths: db3v6, family: ipmetagen.IPv6, destination: "city"},
		{paths: asnv6, family: ipmetagen.IPv6, destination: "asn"},
	} {
		for _, path := range item.paths {
			input, closer, err := openCSV(path, item.family)
			if err != nil {
				return ipmetagen.GenerateOptions{}, closers, err
			}
			closers = append(closers, closer)
			switch item.destination {
			case "country":
				opts.CountryCSVs = append(opts.CountryCSVs, input)
			case "city":
				opts.CityCSVs = append(opts.CityCSVs, input)
			case "asn":
				opts.ASNCSVs = append(opts.ASNCSVs, input)
			}
		}
	}
	return opts, closers, nil
}

func openCSV(source string, family ipmetagen.AddressFamily) (ipmetagen.CSVInput, io.Closer, error) {
	if isURL(source) {
		return openRemoteCSV(source, family)
	}
	if isZIP(source) {
		return openZIPCSV(source, family)
	}
	file, err := os.Open(source)
	if err != nil {
		return ipmetagen.CSVInput{}, nil, fmt.Errorf("open %s: %w", source, err)
	}
	return ipmetagen.CSVInput{Name: source, Family: family, Reader: file}, file, nil
}

func writeGeneratedOutput(outPath string, opts ipmetagen.GenerateOptions) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(outPath), filepath.Base(outPath)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary output for %s: %w", outPath, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if err := ipmetagen.Generate(tmp, opts); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary output %s: %w", tmpPath, err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return fmt.Errorf("chmod temporary output %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("replace output %s: %w", outPath, err)
	}
	return nil
}

func downloadBINData(source string) ([]byte, error) {
	resp, err := getURL(source)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", safeSourceName(source), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("download %s: HTTP %s", safeSourceName(source), resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", safeSourceName(source), err)
	}

	if isZIPBytes(body) {
		reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			return nil, fmt.Errorf("open zip %s: %w", safeSourceName(source), err)
		}
		for _, file := range reader.File {
			if file.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(file.Name), ".bin") {
				continue
			}
			entry, err := file.Open()
			if err != nil {
				return nil, fmt.Errorf("open zip bin %s:%s: %w", safeSourceName(source), file.Name, err)
			}
			defer entry.Close()
			data, err := io.ReadAll(entry)
			if err != nil {
				return nil, fmt.Errorf("read zip bin %s:%s: %w", safeSourceName(source), file.Name, err)
			}
			return data, nil
		}
		return nil, fmt.Errorf("open zip %s: no BIN entry found", safeSourceName(source))
	}
	if len(body) < 64 {
		return nil, fmt.Errorf("download %s did not return a ZIP or BIN payload: %s", safeSourceName(source), responseSnippet(body))
	}
	return body, nil
}

func responseSnippet(body []byte) string {
	text := strings.TrimSpace(string(body))
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 32 {
			return -1
		}
		return r
	}, text)
	if len(text) > 160 {
		text = text[:160] + "..."
	}
	if text == "" {
		return "empty response body"
	}
	return text
}

func ip2locationDownloadURL(token string, fileCode string) string {
	values := url.Values{}
	values.Set("token", token)
	values.Set("file", fileCode)
	return ip2locationDownloadBaseURL + "?" + values.Encode()
}

func openZIPCSV(path string, family ipmetagen.AddressFamily) (ipmetagen.CSVInput, io.Closer, error) {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return ipmetagen.CSVInput{}, nil, fmt.Errorf("open zip %s: %w", path, err)
	}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(file.Name), ".csv") {
			continue
		}
		entry, err := file.Open()
		if err != nil {
			_ = reader.Close()
			return ipmetagen.CSVInput{}, nil, fmt.Errorf("open zip csv %s:%s: %w", path, file.Name, err)
		}
		return ipmetagen.CSVInput{Name: path + ":" + file.Name, Family: family, Reader: entry}, closerFunc(func() error {
			entryErr := entry.Close()
			zipErr := reader.Close()
			if entryErr != nil {
				return entryErr
			}
			return zipErr
		}), nil
	}
	_ = reader.Close()
	return ipmetagen.CSVInput{}, nil, fmt.Errorf("open zip %s: no CSV entry found", path)
}

func openRemoteCSV(source string, family ipmetagen.AddressFamily) (ipmetagen.CSVInput, io.Closer, error) {
	resp, err := getURL(source)
	if err != nil {
		return ipmetagen.CSVInput{}, nil, fmt.Errorf("download %s: %w", safeSourceName(source), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ipmetagen.CSVInput{}, nil, fmt.Errorf("download %s: HTTP %s", safeSourceName(source), resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ipmetagen.CSVInput{}, nil, fmt.Errorf("download %s: %w", safeSourceName(source), err)
	}
	if isZIP(source) || isZIPBytes(body) {
		return openZIPBytes(source, body, family)
	}
	return ipmetagen.CSVInput{Name: safeSourceName(source), Family: family, Reader: bytes.NewReader(body)}, io.NopCloser(bytes.NewReader(nil)), nil
}

func getURL(source string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	return httpClient.Do(req)
}

func isZIPBytes(body []byte) bool {
	return len(body) >= 4 && body[0] == 'P' && body[1] == 'K' && body[2] == 0x03 && body[3] == 0x04
}

func openZIPBytes(source string, body []byte, family ipmetagen.AddressFamily) (ipmetagen.CSVInput, io.Closer, error) {
	reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return ipmetagen.CSVInput{}, nil, fmt.Errorf("open zip %s: %w", safeSourceName(source), err)
	}
	for _, file := range reader.File {
		if file.FileInfo().IsDir() || !strings.HasSuffix(strings.ToLower(file.Name), ".csv") {
			continue
		}
		entry, err := file.Open()
		if err != nil {
			return ipmetagen.CSVInput{}, nil, fmt.Errorf("open zip csv %s:%s: %w", safeSourceName(source), file.Name, err)
		}
		return ipmetagen.CSVInput{Name: safeSourceName(source) + ":" + file.Name, Family: family, Reader: entry}, entry, nil
	}
	return ipmetagen.CSVInput{}, nil, fmt.Errorf("open zip %s: no CSV entry found", safeSourceName(source))
}

func safeSourceName(source string) string {
	parsed, err := url.Parse(source)
	if err != nil || parsed.RawQuery == "" {
		return source
	}
	values := parsed.Query()
	if values.Has("token") {
		values.Set("token", "REDACTED")
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func isURL(source string) bool {
	return strings.HasPrefix(source, "https://") || strings.HasPrefix(source, "http://")
}

func isZIP(source string) bool {
	return strings.HasSuffix(strings.ToLower(source), ".zip")
}

func closeAll(closers []io.Closer) {
	for _, closer := range closers {
		_ = closer.Close()
	}
}

type closerFunc func() error

func (fn closerFunc) Close() error {
	return fn()
}
