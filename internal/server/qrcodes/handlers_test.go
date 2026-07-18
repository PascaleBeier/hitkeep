package qrcodes

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/blocking"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
)

func TestBuildDestinationURLAppliesCampaignParametersAndQRAttribution(t *testing.T) {
	t.Parallel()

	qrID := uuid.MustParse("0f0bbce0-3b6f-4976-9392-95e0a2a7e87f")

	got, err := buildDestinationURL(api.QRCode{
		ID:             qrID,
		DestinationURL: "https://example.com/signup?existing=1&hk_qr=old",
		UTMSource:      "poster",
		UTMMedium:      "print",
		UTMCampaign:    "spring launch",
		UTMContent:     "front window",
		CustomParams:   map[string]string{"region": "berlin", "empty": ""},
	})
	if err != nil {
		t.Fatalf("build destination: %v", err)
	}

	want := "https://example.com/signup?existing=1&hk_qr=0f0bbce0-3b6f-4976-9392-95e0a2a7e87f&region=berlin&utm_campaign=spring+launch&utm_content=front+window&utm_medium=print&utm_source=poster"
	if got != want {
		t.Fatalf("destination mismatch\nwant: %s\n got: %s", want, got)
	}
}

func TestRecordOpenBestEffortDropsPathAndUserAgentExclusions(t *testing.T) {
	tests := []struct {
		name        string
		slug        string
		destination string
		userAgent   string
		rule        database.TrafficExclusionValues
	}{
		{name: "destination path", slug: "path", destination: "https://example.com/admin/users?source=qr#top", userAgent: "Mozilla/5.0", rule: database.TrafficExclusionValues{Type: "path", Path: "/admin"}},
		{name: "request user agent", slug: "ua", destination: "https://example.com/public", userAgent: "QR-BLOCKED-AGENT/1.0", rule: database.TrafficExclusionValues{Type: "user_agent", UserAgent: "qr-blocked-agent"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			store := database.NewStore(":memory:")
			if err := store.Connect(); err != nil {
				t.Fatalf("connect: %v", err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if err := store.Migrate(ctx); err != nil {
				t.Fatalf("migrate: %v", err)
			}
			userID, err := store.CreateUser(ctx, "qr-exclusion-"+test.slug+"@example.test", "hash")
			if err != nil {
				t.Fatalf("create user: %v", err)
			}
			site, err := store.CreateSite(ctx, userID, "qr-exclusion-"+test.slug+".example.test")
			if err != nil {
				t.Fatalf("create site: %v", err)
			}
			qr, _, err := store.CreateQRCode(ctx, site.ID, userID, api.QRCodeCreateRequest{Name: "Excluded QR", DestinationURL: test.destination})
			if err != nil {
				t.Fatalf("create QR code: %v", err)
			}
			if _, err := store.CreateSiteTrafficExclusion(ctx, site.ID, test.rule, userID); err != nil {
				t.Fatalf("create traffic exclusion: %v", err)
			}
			filter := blocking.NewIPFilter(store)
			if err := filter.Refresh(ctx); err != nil {
				t.Fatalf("refresh traffic exclusions: %v", err)
			}
			h := &handler{ctx: &shared.Context{Store: store, Config: &config.Config{}, IPFilter: filter}}
			req := httptest.NewRequest("GET", "/q/token", nil)
			req.RemoteAddr = "198.51.100.22:1234"
			req.Header.Set("User-Agent", test.userAgent)
			h.recordOpenBestEffort(ctx, req, qr)

			count, err := store.CountQRCodeOpens(ctx, site.ID, qr.ID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
			if err != nil {
				t.Fatalf("count QR opens: %v", err)
			}
			if count != 0 {
				t.Fatalf("expected exclusion to suppress QR open, got %d", count)
			}
		})
	}
}

func TestBuildDestinationURLRejectsUnsafeDestinationURLs(t *testing.T) {
	t.Parallel()

	qrID := uuid.MustParse("0f0bbce0-3b6f-4976-9392-95e0a2a7e87f")
	tests := []struct {
		name           string
		destinationURL string
	}{
		{name: "empty", destinationURL: ""},
		{name: "relative path", destinationURL: "/signup"},
		{name: "protocol relative", destinationURL: "//example.com/signup"},
		{name: "unsupported scheme", destinationURL: "ftp://example.com/signup"},
		{name: "javascript scheme", destinationURL: "javascript:alert(1)"},
		{name: "missing host", destinationURL: "https:///signup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := buildDestinationURL(api.QRCode{
				ID:             qrID,
				DestinationURL: tt.destinationURL,
			})
			if err == nil {
				t.Fatalf("expected error for %q, got %q", tt.destinationURL, got)
			}
		})
	}
}
