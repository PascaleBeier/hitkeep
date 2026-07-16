package blocking

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"time"
)

const maxFeedResponseBytes = 10 << 20 // 10 MB

const (
	matomoReferrerSpamURL = "https://raw.githubusercontent.com/matomo-org/referrer-spam-list/master/spammers.txt"
	spamhausDropURL       = "https://www.spamhaus.org/drop/drop_v4.json"
	spamhausDropV6URL     = "https://www.spamhaus.org/drop/drop_v6.json"
)

type spamhausDROPRecord struct {
	Type      string `json:"type"`
	CIDR      string `json:"cidr"`
	Timestamp int64  `json:"timestamp"`
	Records   int    `json:"records"`
	Copyright string `json:"copyright"`
	Terms     string `json:"terms"`
}

type spamhausFeed struct {
	CIDRs    []string
	Metadata SpamFeedSourceMetadata
}

type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

func FetchSpamFeedData(ctx context.Context, client httpDoer) (SpamFeedData, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	var warnings []string

	referrers, err := fetchMatomoReferrers(ctx, client)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("matomo referrer list: %v", err))
	}

	dropv4, err := fetchSpamhausCIDRs(ctx, client, spamhausDropURL, 32)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("spamhaus drop: %v", err))
	}

	dropv6, err := fetchSpamhausCIDRs(ctx, client, spamhausDropV6URL, 128)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("spamhaus dropv6: %v", err))
	}

	if len(referrers) == 0 && len(dropv4.CIDRs) == 0 && len(dropv6.CIDRs) == 0 {
		return SpamFeedData{}, fmt.Errorf("all spam feeds failed: %s", strings.Join(warnings, "; "))
	}

	for _, w := range warnings {
		slog.Warn("Partial spam feed failure, continuing with available data", "error", w)
	}

	sourceMetadata := make(map[string]SpamFeedSourceMetadata, 2)
	if len(dropv4.CIDRs) > 0 {
		sourceMetadata[spamhausDropSource] = dropv4.Metadata
	}
	if len(dropv6.CIDRs) > 0 {
		sourceMetadata[spamhausDropV6Source] = dropv6.Metadata
	}

	data := SpamFeedData{
		GeneratedAt: time.Now().UTC(),
		Sources: map[string]string{
			matomoReferrerSpamSource: matomoReferrerSpamURL,
			spamhausDropSource:       spamhausDropURL,
			spamhausDropV6Source:     spamhausDropV6URL,
		},
		SourceMetadata:       sourceMetadata,
		ReferrerHostDenylist: append([]string(nil), referrers...),
		NetworkDenylist:      append(append([]string(nil), dropv4.CIDRs...), dropv6.CIDRs...),
	}
	data.normalize()
	return data, nil
}

func fetchMatomoReferrers(ctx context.Context, client httpDoer) ([]string, error) {
	body, err := fetchURL(ctx, client, matomoReferrerSpamURL)
	if err != nil {
		return nil, fmt.Errorf("fetch matomo referrer spam list: %w", err)
	}
	defer body.Close()

	var out []string
	scanner := bufio.NewScanner(body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, stripWWW(strings.ToLower(line)))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan matomo referrer spam list: %w", err)
	}
	return normalizeStringList(out), nil
}

func fetchSpamhausCIDRs(ctx context.Context, client httpDoer, sourceURL string, expectedBitLen int) (spamhausFeed, error) {
	body, err := fetchURL(ctx, client, sourceURL)
	if err != nil {
		return spamhausFeed{}, fmt.Errorf("fetch spamhaus cidr list: %w", err)
	}
	defer body.Close()

	if expectedBitLen != 32 && expectedBitLen != 128 {
		return spamhausFeed{}, fmt.Errorf("unsupported spamhaus address size %d", expectedBitLen)
	}
	expectedFamily := "IPv4"
	if expectedBitLen == 128 {
		expectedFamily = "IPv6"
	}

	decoder := json.NewDecoder(body)
	var (
		out             []string
		metadata        SpamFeedSourceMetadata
		declaredRecords int
		seenMetadata    bool
	)
	for recordNumber := 1; ; recordNumber++ {
		var record spamhausDROPRecord
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			return spamhausFeed{}, fmt.Errorf("decode spamhaus JSON record %d: %w", recordNumber, err)
		}

		if seenMetadata {
			if record.Type == "metadata" {
				return spamhausFeed{}, fmt.Errorf("spamhaus JSON contains duplicate metadata records")
			}
			return spamhausFeed{}, fmt.Errorf("spamhaus JSON metadata record must be terminal")
		}

		if record.Type != "" {
			if record.Type != "metadata" {
				return spamhausFeed{}, fmt.Errorf("spamhaus JSON contains unknown record type %q", record.Type)
			}
			if record.Timestamp <= 0 {
				return spamhausFeed{}, fmt.Errorf("spamhaus JSON metadata is missing timestamp")
			}
			if strings.TrimSpace(record.Copyright) == "" {
				return spamhausFeed{}, fmt.Errorf("spamhaus JSON metadata is missing copyright")
			}
			if strings.TrimSpace(record.Terms) == "" {
				return spamhausFeed{}, fmt.Errorf("spamhaus JSON metadata is missing terms")
			}
			metadata = SpamFeedSourceMetadata{
				Timestamp: record.Timestamp,
				Copyright: record.Copyright,
				Terms:     record.Terms,
			}
			declaredRecords = record.Records
			seenMetadata = true
			continue
		}

		cidr := strings.TrimSpace(record.CIDR)
		if cidr == "" {
			return spamhausFeed{}, fmt.Errorf("spamhaus JSON record %d is missing cidr", recordNumber)
		}
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return spamhausFeed{}, fmt.Errorf("spamhaus JSON record %d has invalid cidr %q: %w", recordNumber, cidr, err)
		}
		if prefix.Addr().BitLen() != expectedBitLen {
			return spamhausFeed{}, fmt.Errorf("spamhaus JSON record %d contains %s cidr %q in %s feed", recordNumber, addressFamilyName(prefix), cidr, expectedFamily)
		}
		out = append(out, cidr)
	}
	if !seenMetadata {
		return spamhausFeed{}, fmt.Errorf("spamhaus JSON is missing terminal metadata record")
	}
	if len(out) == 0 {
		return spamhausFeed{}, fmt.Errorf("spamhaus JSON contains no CIDR records")
	}
	if declaredRecords != len(out) {
		return spamhausFeed{}, fmt.Errorf("spamhaus JSON declares %d records but contains %d", declaredRecords, len(out))
	}
	return spamhausFeed{CIDRs: normalizeStringList(out), Metadata: metadata}, nil
}

func addressFamilyName(prefix netip.Prefix) string {
	if prefix.Addr().Is4() {
		return "IPv4"
	}
	return "IPv6"
}

func fetchURL(ctx context.Context, client httpDoer, sourceURL string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	return io.NopCloser(io.LimitReader(resp.Body, maxFeedResponseBytes)), nil
}
