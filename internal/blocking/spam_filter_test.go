package blocking

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
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
	filter := NewSpamFilter("", testBlockingLogger())
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

func TestNewSpamFilterRequiresLogger(t *testing.T) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected missing logger panic")
		}
	}()
	_ = NewSpamFilter("", nil)
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
	if diff := cmp.Diff(input.SourceMetadata, loaded.SourceMetadata); diff != "" {
		t.Fatalf("unexpected source metadata (-want +got):\n%s", diff)
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
	failures  map[string]error
}

func (f fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if err, ok := f.failures[req.URL.String()]; ok {
		return nil, err
	}
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

	data, err := FetchSpamFeedData(context.Background(), client, testBlockingLogger())
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

func testBlockingLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestFetchSpamFeedDataPartialFailure(t *testing.T) {
	client := fakeHTTPClient{
		responses: map[string]string{
			matomoReferrerSpamURL: "spam.example\n",
			// spamhausDropURL and spamhausDropV6URL are missing → 404
		},
	}

	data, err := FetchSpamFeedData(context.Background(), client, testBlockingLogger())
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

func TestFetchSpamFeedDataDoesNotLogRawFailureDetails(t *testing.T) {
	const rawFailure = "provider response password=do-not-log"
	client := fakeHTTPClient{
		responses: map[string]string{
			matomoReferrerSpamURL: "spam.example\n",
		},
		failures: map[string]error{
			spamhausDropURL: fmt.Errorf("upstream returned %s", rawFailure),
		},
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	if _, err := FetchSpamFeedData(context.Background(), client, logger); err != nil {
		t.Fatalf("partial failure should not return error: %v", err)
	}
	if strings.Contains(logs.String(), rawFailure) {
		t.Fatalf("raw upstream failure appeared in logs: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "source=spamhaus_drop_v4") || !strings.Contains(logs.String(), "error_kind=invalid_response") {
		t.Fatalf("expected safe source and error kind in logs: %s", logs.String())
	}
}

func TestFetchSpamFeedDataAllFeedsFail(t *testing.T) {
	const rawFailure = "provider response password=do-not-return"
	client := fakeHTTPClient{
		responses: map[string]string{},
		failures: map[string]error{
			matomoReferrerSpamURL: fmt.Errorf("upstream returned %s", rawFailure),
		},
	}

	_, err := FetchSpamFeedData(context.Background(), client, testBlockingLogger())
	if err == nil {
		t.Fatal("expected error when all feeds fail")
	}
	if !strings.Contains(err.Error(), "all spam feeds failed") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if strings.Contains(err.Error(), rawFailure) {
		t.Fatalf("raw upstream failure appeared in returned error: %v", err)
	}
}

type trackedReadCloser struct {
	reader io.Reader
	closed bool
}

func (r *trackedReadCloser) Read(p []byte) (int, error) { return r.reader.Read(p) }
func (r *trackedReadCloser) Close() error {
	r.closed = true
	return nil
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type singleResponseClient struct {
	response *http.Response
}

func (c singleResponseClient) Do(*http.Request) (*http.Response, error) { return c.response, nil }

func TestFetchURLClosesResponseBodiesAndRejectsOversize(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		reader     io.Reader
		wantErr    string
		wantBody   string
	}{
		{name: "success", statusCode: http.StatusOK, reader: strings.NewReader("ok"), wantBody: "ok"},
		{name: "status failure", statusCode: http.StatusBadGateway, reader: strings.NewReader("failure"), wantErr: "unexpected HTTP status"},
		{name: "read failure", statusCode: http.StatusOK, reader: failingReader{}, wantErr: "unexpected EOF"},
		{name: "oversize", statusCode: http.StatusOK, reader: strings.NewReader(strings.Repeat("x", maxFeedResponseBytes+1)), wantErr: "response body exceeds"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			body := &trackedReadCloser{reader: tc.reader}
			got, err := fetchURL(context.Background(), singleResponseClient{response: &http.Response{StatusCode: tc.statusCode, Body: body}}, "https://example.com/feed")
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("fetch URL: %v", err)
				}
				raw, err := io.ReadAll(got)
				if err != nil {
					t.Fatalf("read bounded response: %v", err)
				}
				if string(raw) != tc.wantBody {
					t.Fatalf("expected body %q, got %q", tc.wantBody, raw)
				}
			}
			if !body.closed {
				t.Fatal("expected original response body to close")
			}
		})
	}

	exact := &trackedReadCloser{reader: strings.NewReader(strings.Repeat("x", maxFeedResponseBytes))}
	got, err := fetchURL(context.Background(), singleResponseClient{response: &http.Response{StatusCode: http.StatusOK, Body: exact}}, "https://example.com/feed")
	if err != nil {
		t.Fatalf("fetch exactly capped response: %v", err)
	}
	raw, err := io.ReadAll(got)
	if err != nil || len(raw) != maxFeedResponseBytes {
		t.Fatalf("expected exactly %d readable bytes, got %d, %v", maxFeedResponseBytes, len(raw), err)
	}
	if !exact.closed {
		t.Fatal("expected exact-cap response body to close")
	}
}

func TestFetchSpamhausCIDRsClosesBodyAfterParserFailure(t *testing.T) {
	body := &trackedReadCloser{reader: strings.NewReader(`{"cidr":`)}
	_, err := fetchSpamhausCIDRs(context.Background(), singleResponseClient{response: &http.Response{StatusCode: http.StatusOK, Body: body}}, "https://example.com/feed", 32)
	if err == nil || !strings.Contains(err.Error(), "decode spamhaus JSON") {
		t.Fatalf("expected parser failure, got %v", err)
	}
	if !body.closed {
		t.Fatal("expected parser failure response body to close")
	}
}

func TestSaveSpamFeedDataIsAtomicAndPreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spam-filter.json")
	old := completeTestSpamFeedData()
	old.ReferrerHostDenylist = []string{"old.example"}
	if err := SaveSpamFeedData(path, old); err != nil {
		t.Fatalf("save old cache: %v", err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatalf("set old cache mode: %v", err)
	}

	updated := completeTestSpamFeedData()
	updated.ReferrerHostDenylist = []string{"new.example"}
	if err := SaveSpamFeedData(path, updated); err != nil {
		t.Fatalf("replace old cache: %v", err)
	}
	loaded, err := LoadSpamFeedData(path)
	if err != nil {
		t.Fatalf("load replaced cache: %v", err)
	}
	if diff := cmp.Diff(updated.ReferrerHostDenylist, loaded.ReferrerHostDenylist); diff != "" {
		t.Fatalf("cache was not replaced atomically (-want +got):\n%s", diff)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat replaced cache: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o640 {
		t.Fatalf("expected existing cache mode 0640 to survive, got %04o", got)
	}

	blocked := filepath.Join(dir, "blocked-cache")
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatalf("make blocked target: %v", err)
	}
	sentinel := filepath.Join(blocked, "old-cache")
	if err := os.WriteFile(sentinel, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old target marker: %v", err)
	}
	if err := SaveSpamFeedData(blocked, updated); err == nil {
		t.Fatal("expected replacement of directory target to fail")
	}
	if raw, err := os.ReadFile(sentinel); err != nil || string(raw) != "old" {
		t.Fatalf("existing target changed after failed replacement: %q, %v", raw, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read cache directory: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".spam-filter-") {
			t.Fatalf("temporary cache file %q was not cleaned up", entry.Name())
		}
	}
}

func TestSaveSpamFeedDataWritesNewFile0600AndReplacesFinalSymlink(t *testing.T) {
	dir := t.TempDir()
	data := completeTestSpamFeedData()
	data.ReferrerHostDenylist = []string{"replacement.example"}

	newPath := filepath.Join(dir, "new-cache.json")
	if err := SaveSpamFeedData(newPath, data); err != nil {
		t.Fatalf("save new cache: %v", err)
	}
	info, err := os.Stat(newPath)
	if err != nil {
		t.Fatalf("stat new cache: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected new cache mode 0600, got %04o", got)
	}

	formerTarget := filepath.Join(dir, "former-target.json")
	if err := os.WriteFile(formerTarget, []byte("unchanged"), 0o600); err != nil {
		t.Fatalf("write former symlink target: %v", err)
	}
	linkPath := filepath.Join(dir, "cache-link.json")
	if err := os.Symlink(formerTarget, linkPath); err != nil {
		t.Skipf("create final cache symlink: %v", err)
	}
	if err := SaveSpamFeedData(linkPath, data); err != nil {
		t.Fatalf("replace final cache symlink: %v", err)
	}
	linkInfo, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("lstat replaced symlink path: %v", err)
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		t.Fatal("final cache symlink was followed instead of replaced")
	}
	if got := linkInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("expected replacement cache mode 0600, got %04o", got)
	}
	if raw, err := os.ReadFile(formerTarget); err != nil || string(raw) != "unchanged" {
		t.Fatalf("former symlink target changed: %q, %v", raw, err)
	}
	loaded, err := LoadSpamFeedData(linkPath)
	if err != nil {
		t.Fatalf("load replacement cache: %v", err)
	}
	if diff := cmp.Diff(data.ReferrerHostDenylist, loaded.ReferrerHostDenylist); diff != "" {
		t.Fatalf("unexpected replacement cache data (-want +got):\n%s", diff)
	}
}

func TestSpamFilterSerializesWholeUpdateGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spam-filter.json")
	filter := NewSpamFilter(path, testBlockingLogger())
	first := completeTestSpamFeedData()
	first.ReferrerHostDenylist = []string{"first.example"}
	second := completeTestSpamFeedData()
	second.ReferrerHostDenylist = []string{"second.example"}

	firstFetched := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondFetched := make(chan struct{})
	var fetchMu sync.Mutex
	fetchCount := 0
	filter.fetch = func(context.Context) (SpamFeedData, error) {
		fetchMu.Lock()
		fetchCount++
		call := fetchCount
		fetchMu.Unlock()
		if call == 1 {
			close(firstFetched)
			<-releaseFirst
			return first, nil
		}
		close(secondFetched)
		return second, nil
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- filter.Update(context.Background()) }()
	select {
	case <-firstFetched:
	case <-time.After(time.Second):
		t.Fatal("first update did not begin its fetch")
	}

	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		secondDone <- filter.Update(context.Background())
	}()
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second update did not start")
	}
	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("finish first update: %v", err)
	}
	select {
	case <-secondFetched:
	case <-time.After(time.Second):
		t.Fatal("second update did not fetch after the first transition completed")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("finish second update: %v", err)
	}

	fromDisk, err := LoadSpamFeedData(path)
	if err != nil {
		t.Fatalf("load final cache: %v", err)
	}
	filter.mu.RLock()
	inMemory := filter.data
	filter.mu.RUnlock()
	expected := second
	expected.normalize()
	if diff := cmp.Diff(expected, fromDisk); diff != "" {
		t.Fatalf("older update committed after newer result (-want +disk):\n%s", diff)
	}
	if diff := cmp.Diff(fromDisk, inMemory); diff != "" {
		t.Fatalf("disk and memory generations interleaved (-disk +memory):\n%s", diff)
	}
}

func TestSpamFilterQueuedUpdateCancellationDoesNotFetchOrCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "spam-filter.json")
	filter := NewSpamFilter(path, testBlockingLogger())
	first := completeTestSpamFeedData()
	first.ReferrerHostDenylist = []string{"first.example"}
	firstFetched := make(chan struct{})
	releaseFirst := make(chan struct{})
	var fetchMu sync.Mutex
	fetchCount := 0
	filter.fetch = func(context.Context) (SpamFeedData, error) {
		fetchMu.Lock()
		fetchCount++
		fetchMu.Unlock()
		close(firstFetched)
		<-releaseFirst
		return first, nil
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- filter.Update(context.Background()) }()
	select {
	case <-firstFetched:
	case <-time.After(time.Second):
		t.Fatal("first update did not begin its fetch")
	}

	secondCtx, cancelSecond := context.WithCancel(context.Background())
	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		secondDone <- filter.Update(secondCtx)
	}()
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("second update did not start")
	}
	cancelSecond()
	select {
	case err := <-secondDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued update error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued canceled update did not return")
	}
	fetchMu.Lock()
	gotFetches := fetchCount
	fetchMu.Unlock()
	if gotFetches != 1 {
		t.Fatalf("queued canceled update invoked fetch %d times, want 1", gotFetches)
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("finish first update: %v", err)
	}
	fromDisk, err := LoadSpamFeedData(path)
	if err != nil {
		t.Fatalf("load final cache: %v", err)
	}
	expected := first
	expected.normalize()
	if diff := cmp.Diff(expected, fromDisk); diff != "" {
		t.Fatalf("queued canceled update committed data (-want +disk):\n%s", diff)
	}
}

func TestSpamFilterQueuedTransitionCancellationReleasesGate(t *testing.T) {
	filter := NewSpamFilter(filepath.Join(t.TempDir(), "spam-filter.json"), testBlockingLogger())

	<-filter.transition

	ctx, cancel := context.WithCancel(context.Background())
	acquired := make(chan error, 1)
	go func() {
		acquired <- filter.acquireTransition(ctx)
	}()

	cancel()
	select {
	case err := <-acquired:
		if err != context.Canceled {
			t.Fatalf("queued transition acquisition error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued transition acquisition did not return after cancellation")
	}

	filter.transition <- struct{}{}
	if err := filter.acquireTransition(context.Background()); err != nil {
		t.Fatalf("transition acquisition after release: %v", err)
	}
	filter.transition <- struct{}{}
}

func TestSpamFilterInitialRefreshIsAsyncAndCancellationAware(t *testing.T) {
	filter := NewSpamFilter("", testBlockingLogger())
	started := make(chan struct{})
	stopped := make(chan struct{})
	filter.fetch = func(ctx context.Context) (SpamFeedData, error) {
		close(started)
		<-ctx.Done()
		close(stopped)
		return SpamFeedData{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	returned := make(chan struct{})
	go func() {
		filter.StartRefreshLoop(ctx, true, time.Hour, func() bool { return true })
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("initial refresh blocked startup")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial refresh did not start asynchronously")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("cancellation did not reach initial refresh")
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
			name:           "duplicate object name",
			body:           `{"cidr":"203.0.113.0/24","cidr":"198.51.100.0/24"}` + "\n" + metadata,
			expectedBitLen: 32,
			wantErr:        "decode spamhaus JSON",
		},
		{
			name:           "invalid UTF-8",
			body:           string(append([]byte(`{"cidr":"`), append([]byte{0xff}, []byte(`"}`)...)...)),
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
