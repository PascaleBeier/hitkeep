package blocking

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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

func TestFetchSpamFeedData(t *testing.T) {
	client := fakeHTTPClient{
		responses: map[string]string{
			matomoReferrerSpamURL: "spam.example\nwww.bad.example\n",
			spamhausDropURL:       "; header\n203.0.113.0/24 ; SBL1\n",
			spamhausDropV6URL:     "; header\n2001:db8::/32 ; SBL2\n",
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
