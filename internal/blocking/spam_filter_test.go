package blocking

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeReferrerHost(t *testing.T) {
	tests := []struct {
		name     string
		referrer *string
		want     string
	}{
		{
			name:     "nil referrer",
			referrer: nil,
			want:     "",
		},
		{
			name:     "blank referrer",
			referrer: new("   "),
			want:     "",
		},
		{
			name:     "url with port query and uppercase",
			referrer: new(" HTTPS://WWW.Spam.Example:8443/path?q=1 "),
			want:     "spam.example",
		},
		{
			name:     "plain hostname with slashes",
			referrer: new("www.buttons-for-website.example///"),
			want:     "buttons-for-website.example",
		},
		{
			name:     "hostname without scheme keeps host",
			referrer: new("semalt.example"),
			want:     "semalt.example",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeReferrerHost(tc.referrer); got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestIsSameSiteHost(t *testing.T) {
	tests := []struct {
		name         string
		referrerHost string
		siteDomain   string
		want         bool
	}{
		{
			name:         "exact same host",
			referrerHost: "example.com",
			siteDomain:   "example.com",
			want:         true,
		},
		{
			name:         "subdomain of site",
			referrerHost: "docs.example.com",
			siteDomain:   "example.com",
			want:         true,
		},
		{
			name:         "www site domain normalization",
			referrerHost: "blog.example.com",
			siteDomain:   "www.example.com",
			want:         true,
		},
		{
			name:         "different domain",
			referrerHost: "spam.example",
			siteDomain:   "example.com",
			want:         false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSameSiteHost(tc.referrerHost, tc.siteDomain); got != tc.want {
				t.Fatalf("expected %t, got %t", tc.want, got)
			}
		})
	}
}

func TestSpamFilterEvaluate(t *testing.T) {
	filter := NewSpamFilter("")
	filter.apply(SpamFeedData{
		ReferrerHostDenylist: []string{
			"buttons-for-website.example",
			"seo-audit.example",
			"example.com",
		},
		NetworkDenylist: []string{
			"203.0.113.0/24",
			"2001:db8:dead::/48",
			"invalid-cidr",
		},
	})

	tests := []struct {
		name       string
		siteDomain string
		userIP     string
		referrer   *string
		want       SpamDecision
	}{
		{
			name:       "blocks known spam referrer url",
			siteDomain: "site.example",
			userIP:     "198.51.100.10",
			referrer:   new("https://www.buttons-for-website.example/landing?campaign=1"),
			want:       SpamDecision{Blocked: true, Reason: "matomo_referrer_spam"},
		},
		{
			name:       "blocks plain spam referrer host",
			siteDomain: "site.example",
			userIP:     "198.51.100.10",
			referrer:   new("seo-audit.example///"),
			want:       SpamDecision{Blocked: true, Reason: "matomo_referrer_spam"},
		},
		{
			name:       "allows same-site referrer even if denylisted",
			siteDomain: "example.com",
			userIP:     "198.51.100.10",
			referrer:   new("https://www.example.com/docs/getting-started"),
			want:       SpamDecision{},
		},
		{
			name:       "allows same-site subdomain referrer",
			siteDomain: "example.com",
			userIP:     "198.51.100.10",
			referrer:   new("https://blog.example.com/post"),
			want:       SpamDecision{},
		},
		{
			name:       "blocks spamhaus ipv4 network before referrer checks",
			siteDomain: "example.com",
			userIP:     "203.0.113.5",
			referrer:   new("https://www.example.com/internal"),
			want:       SpamDecision{Blocked: true, Reason: "spamhaus_drop"},
		},
		{
			name:       "blocks spamhaus ipv6 network",
			siteDomain: "example.com",
			userIP:     "2001:db8:dead::1",
			referrer:   nil,
			want:       SpamDecision{Blocked: true, Reason: "spamhaus_drop"},
		},
		{
			name:       "ignores invalid client ip",
			siteDomain: "site.example",
			userIP:     "not-an-ip",
			referrer:   nil,
			want:       SpamDecision{},
		},
		{
			name:       "allows direct traffic",
			siteDomain: "site.example",
			userIP:     "198.51.100.10",
			referrer:   nil,
			want:       SpamDecision{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := filter.Evaluate(tc.siteDomain, tc.userIP, tc.referrer)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("expected %+v, got %+v", tc.want, got)
			}
		})
	}
}

func TestSaveAndLoadSpamFeedData(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "spam-filter.json")
	input := SpamFeedData{
		GeneratedAt: time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC),
		Sources: map[string]string{
			matomoReferrerSpamSource: matomoReferrerSpamURL,
			spamhausDropSource:       spamhausDropURL,
			spamhausDropV6Source:     spamhausDropV6URL,
		},
		SourceMetadata: map[string]SpamFeedSourceMetadata{
			spamhausDropSource: {
				Timestamp: 1_784_054_642,
				Copyright: "(c) 2026 The Spamhaus Project SLU",
				Terms:     "https://www.spamhaus.org/drop/terms/",
			},
		},
		ReferrerHostDenylist: []string{"b.example", "a.example"},
		NetworkDenylist:      []string{"203.0.113.0/24"},
	}

	if err := SaveSpamFeedData(path, input); err != nil {
		t.Fatalf("save spam feed data: %v", err)
	}
	loaded, err := LoadSpamFeedData(path)
	if err != nil {
		t.Fatalf("load spam feed data: %v", err)
	}
	if len(loaded.ReferrerHostDenylist) != 2 || loaded.ReferrerHostDenylist[0] != "a.example" {
		t.Fatalf("unexpected referrer list: %+v", loaded.ReferrerHostDenylist)
	}
	if len(loaded.NetworkDenylist) != 1 || loaded.NetworkDenylist[0] != "203.0.113.0/24" {
		t.Fatalf("unexpected network list: %+v", loaded.NetworkDenylist)
	}
	if !reflect.DeepEqual(loaded.SourceMetadata, input.SourceMetadata) {
		t.Fatalf("unexpected source metadata: %+v", loaded.SourceMetadata)
	}
}

func TestDecodeSpamFeedDataAcceptsLegacyCacheWithoutSourceMetadata(t *testing.T) {
	data, err := decodeSpamFeedData([]byte(`{
  "generated_at": "2026-07-16T12:00:00Z",
  "sources": {"legacy": "https://example.com/list.txt"},
  "referrer_host_denylist": ["spam.example"],
  "network_denylist": ["203.0.113.0/24"]
}`))
	if err != nil {
		t.Fatalf("decode legacy cache: %v", err)
	}
	if data.SourceMetadata != nil {
		t.Fatalf("expected absent source metadata to remain nil, got %+v", data.SourceMetadata)
	}
}

func TestValidateEmbeddedSpamFeedData(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*SpamFeedData)
		wantErr string
	}{
		{name: "complete"},
		{
			name: "missing referrers",
			mutate: func(data *SpamFeedData) {
				data.ReferrerHostDenylist = nil
			},
			wantErr: "no referrer hosts",
		},
		{
			name: "missing IPv4",
			mutate: func(data *SpamFeedData) {
				data.NetworkDenylist = []string{"2001:db8::/32"}
			},
			wantErr: "no IPv4 networks",
		},
		{
			name: "missing IPv6",
			mutate: func(data *SpamFeedData) {
				data.NetworkDenylist = []string{"203.0.113.0/24"}
			},
			wantErr: "no IPv6 networks",
		},
		{
			name: "invalid network",
			mutate: func(data *SpamFeedData) {
				data.NetworkDenylist = []string{"invalid", "2001:db8::/32"}
			},
			wantErr: "invalid network",
		},
		{
			name: "missing attribution",
			mutate: func(data *SpamFeedData) {
				delete(data.SourceMetadata, spamhausDropV6Source)
			},
			wantErr: "missing metadata",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := completeTestSpamFeedData()
			if tc.mutate != nil {
				tc.mutate(&data)
			}
			err := ValidateEmbeddedSpamFeedData(data)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("validate complete embedded data: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestEmbeddedSpamFeedDataIsComplete(t *testing.T) {
	data, err := LoadEmbeddedSpamFeedData()
	if err != nil {
		t.Fatalf("load embedded spam data: %v", err)
	}
	if err := ValidateEmbeddedSpamFeedData(data); err != nil {
		t.Fatalf("validate embedded spam data: %v", err)
	}
}

type fakeHTTPClient struct {
	responses map[string]string
}

func (f fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	body, ok := f.responses[req.URL.String()]
	if !ok {
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Status:     "404 Not Found",
			Body:       io.NopCloser(strings.NewReader("missing")),
			Header:     make(http.Header),
		}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}, nil
}

func completeTestSpamFeedData() SpamFeedData {
	return SpamFeedData{
		GeneratedAt: time.Date(2026, time.July, 16, 12, 0, 0, 0, time.UTC),
		Sources: map[string]string{
			matomoReferrerSpamSource: matomoReferrerSpamURL,
			spamhausDropSource:       spamhausDropURL,
			spamhausDropV6Source:     spamhausDropV6URL,
		},
		SourceMetadata: map[string]SpamFeedSourceMetadata{
			spamhausDropSource: {
				Timestamp: 1_784_054_642,
				Copyright: "(c) 2026 The Spamhaus Project SLU",
				Terms:     "https://www.spamhaus.org/drop/terms/",
			},
			spamhausDropV6Source: {
				Timestamp: 1_784_164_442,
				Copyright: "(c) 2026 The Spamhaus Project SLU",
				Terms:     "https://www.spamhaus.org/drop/terms/",
			},
		},
		ReferrerHostDenylist: []string{"spam.example"},
		NetworkDenylist:      []string{"203.0.113.0/24", "2001:db8::/32"},
	}
}

func spamhausJSONFeed(cidr string, timestamp int64) string {
	return fmt.Sprintf(
		"{\"cidr\":%q,\"sblid\":\"SBL1\",\"rir\":\"test\"}\n"+
			"{\"type\":\"metadata\",\"timestamp\":%d,\"size\":100,\"records\":1,\"copyright\":\"(c) 2026 The Spamhaus Project SLU\",\"terms\":\"https://www.spamhaus.org/drop/terms/\"}\n",
		cidr,
		timestamp,
	)
}

func TestFetchSpamFeedData(t *testing.T) {
	client := fakeHTTPClient{
		responses: map[string]string{
			matomoReferrerSpamURL: "spam.example\nwww.bad.example\n",
			spamhausDropURL:       spamhausJSONFeed("203.0.113.0/24", 1_784_054_642),
			spamhausDropV6URL:     spamhausJSONFeed("2001:db8::/32", 1_784_164_442),
		},
	}

	data, err := FetchSpamFeedData(context.Background(), client)
	if err != nil {
		t.Fatalf("fetch spam feed data: %v", err)
	}
	if got := data.ReferrerHostDenylist; len(got) != 2 || got[0] != "bad.example" || got[1] != "spam.example" {
		t.Fatalf("unexpected referrer hosts: %+v", got)
	}
	if got := data.NetworkDenylist; len(got) != 2 {
		t.Fatalf("unexpected network denylist: %+v", got)
	}
	if got := data.SourceMetadata[spamhausDropSource]; got.Timestamp != 1_784_054_642 || got.Copyright != "(c) 2026 The Spamhaus Project SLU" || got.Terms != "https://www.spamhaus.org/drop/terms/" {
		t.Fatalf("unexpected IPv4 source metadata: %+v", got)
	}
	if got := data.SourceMetadata[spamhausDropV6Source]; got.Timestamp != 1_784_164_442 {
		t.Fatalf("unexpected IPv6 source metadata: %+v", got)
	}
}

func TestFetchSpamFeedDataPartialFailure(t *testing.T) {
	client := fakeHTTPClient{
		responses: map[string]string{
			matomoReferrerSpamURL: "spam.example\n",
			// spamhausDropURL and spamhausDropV6URL are missing → 404
		},
	}

	data, err := FetchSpamFeedData(context.Background(), client)
	if err != nil {
		t.Fatalf("partial failure should not return error, got: %v", err)
	}
	if len(data.ReferrerHostDenylist) != 1 || data.ReferrerHostDenylist[0] != "spam.example" {
		t.Fatalf("unexpected referrer hosts: %+v", data.ReferrerHostDenylist)
	}
	if len(data.NetworkDenylist) != 0 {
		t.Fatalf("expected empty network denylist, got: %+v", data.NetworkDenylist)
	}
	if len(data.SourceMetadata) != 0 {
		t.Fatalf("expected no source metadata for failed Spamhaus feeds, got: %+v", data.SourceMetadata)
	}
}

func TestFetchSpamFeedDataAllFeedsFail(t *testing.T) {
	client := fakeHTTPClient{
		responses: map[string]string{},
	}

	_, err := FetchSpamFeedData(context.Background(), client)
	if err == nil {
		t.Fatal("expected error when all feeds fail")
	}
	if !strings.Contains(err.Error(), "all spam feeds failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestFetchSpamhausCIDRsRejectsInvalidJSONFeeds(t *testing.T) {
	metadata := `{"type":"metadata","timestamp":1784054642,"records":1,"copyright":"(c) 2026 The Spamhaus Project SLU","terms":"https://www.spamhaus.org/drop/terms/"}` + "\n"
	tests := []struct {
		name           string
		body           string
		expectedBitLen int
		wantErr        string
	}{
		{
			name:           "malformed JSON",
			body:           `{"cidr":`,
			expectedBitLen: 32,
			wantErr:        "decode spamhaus JSON",
		},
		{
			name:           "invalid CIDR",
			body:           `{"cidr":"invalid"}` + "\n" + metadata,
			expectedBitLen: 32,
			wantErr:        "invalid cidr",
		},
		{
			name:           "wrong address family",
			body:           spamhausJSONFeed("2001:db8::/32", 1_784_054_642),
			expectedBitLen: 32,
			wantErr:        "IPv6 cidr",
		},
		{
			name:           "missing metadata",
			body:           `{"cidr":"203.0.113.0/24"}` + "\n",
			expectedBitLen: 32,
			wantErr:        "missing terminal metadata",
		},
		{
			name: "duplicate metadata",
			body: spamhausJSONFeed("203.0.113.0/24", 1_784_054_642) +
				`{"type":"metadata","timestamp":1784054643,"records":1,"copyright":"(c) 2026 The Spamhaus Project SLU","terms":"https://www.spamhaus.org/drop/terms/"}` + "\n",
			expectedBitLen: 32,
			wantErr:        "duplicate metadata",
		},
		{
			name: "metadata not terminal",
			body: metadata +
				`{"cidr":"203.0.113.0/24"}` + "\n",
			expectedBitLen: 32,
			wantErr:        "must be terminal",
		},
		{
			name: "record count mismatch",
			body: `{"cidr":"203.0.113.0/24"}` + "\n" +
				`{"type":"metadata","timestamp":1784054642,"records":2,"copyright":"(c) 2026 The Spamhaus Project SLU","terms":"https://www.spamhaus.org/drop/terms/"}` + "\n",
			expectedBitLen: 32,
			wantErr:        "declares 2 records but contains 1",
		},
		{
			name: "missing attribution",
			body: `{"cidr":"203.0.113.0/24"}` + "\n" +
				`{"type":"metadata","timestamp":1784054642,"records":1}` + "\n",
			expectedBitLen: 32,
			wantErr:        "missing copyright",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := fakeHTTPClient{responses: map[string]string{spamhausDropURL: tc.body}}
			_, err := fetchSpamhausCIDRs(context.Background(), client, spamhausDropURL, tc.expectedBitLen)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
