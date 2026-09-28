package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
)

func TestQRCodeStorePersistsDefinitionsAssetsSharesAndStats(t *testing.T) {
	store, userID, site := setupAppenderStore(t)
	ctx := context.Background()

	qr, token, err := store.CreateQRCode(ctx, site.ID, userID, api.QRCodeCreateRequest{
		Name:           "Spring poster",
		DestinationURL: "https://example.com/signup",
		UTMSource:      "poster",
		UTMMedium:      "print",
		UTMCampaign:    "spring",
		CustomParams:   map[string]string{"region": "berlin"},
		Style:          map[string]any{"foreground": "#111827", "dots": "rounded"},
	})
	if err != nil {
		t.Fatalf("create QR code: %v", err)
	}
	if token == "" || qr.RedirectToken != token {
		t.Fatalf("expected persisted redirect token, got token=%q qr token=%q", token, qr.RedirectToken)
	}

	found, err := store.GetQRCodeByToken(ctx, token)
	if err != nil {
		t.Fatalf("get QR by token: %v", err)
	}
	if found == nil || found.ID != qr.ID || found.CustomParams["region"] != "berlin" {
		t.Fatalf("unexpected QR lookup: %+v", found)
	}

	asset, err := store.UpsertQRCodeAsset(ctx, api.QRCodeAsset{
		QRCodeID:    qr.ID,
		SiteID:      site.ID,
		Filename:    "logo.png",
		ContentType: "image/png",
		ByteSize:    4,
		Checksum:    "checksum",
		Data:        []byte{0x89, 'P', 'N', 'G'},
	})
	if err != nil {
		t.Fatalf("upsert QR asset: %v", err)
	}
	if asset == nil || asset.Filename != "logo.png" {
		t.Fatalf("unexpected asset: %+v", asset)
	}

	share, shareToken, err := store.CreateQRCodeShareLink(ctx, site.ID, qr.ID, userID)
	if err != nil {
		t.Fatalf("create QR share: %v", err)
	}
	if share == nil || shareToken == "" {
		t.Fatalf("expected QR share and token, got share=%+v token=%q", share, shareToken)
	}
	sharedQR, err := store.GetQRCodeByShareToken(ctx, shareToken)
	if err != nil {
		t.Fatalf("get QR by share token: %v", err)
	}
	if sharedQR == nil || sharedQR.ID != qr.ID {
		t.Fatalf("unexpected shared QR lookup: %+v", sharedQR)
	}

	now := time.Now().UTC()
	if err := store.CreateQRCodeOpen(ctx, &api.QRCodeOpen{SiteID: site.ID, QRCodeID: qr.ID, Timestamp: now}); err != nil {
		t.Fatalf("create QR open: %v", err)
	}
	count, err := store.CountQRCodeOpens(ctx, site.ID, qr.ID, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil {
		t.Fatalf("count QR opens: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 QR open, got %d", count)
	}
	series, err := store.GetQRCodeOpenSeries(ctx, site.ID, qr.ID, now.Add(-90*time.Minute), now.Add(90*time.Minute))
	if err != nil {
		t.Fatalf("get QR open series: %v", err)
	}
	seriesOpens := 0
	for _, point := range series {
		seriesOpens += point.Opens
	}
	if seriesOpens != 1 {
		t.Fatalf("expected QR open series to include 1 open, got %d from %+v", seriesOpens, series)
	}

	otherQRID := uuid.New()
	if err := store.CreateHitsBulk(ctx, []*api.Hit{
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: now, Path: "/qr", QRCodeID: &qr.ID},
		{SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: now, Path: "/other", QRCodeID: &otherQRID},
	}); err != nil {
		t.Fatalf("create hits: %v", err)
	}
	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID:  site.ID,
		UserID:  userID,
		Start:   now.Add(-time.Hour),
		End:     now.Add(time.Hour),
		Filters: []api.Filter{{Type: "qr_code_id", Value: qr.ID.String()}},
	})
	if err != nil {
		t.Fatalf("get filtered stats: %v", err)
	}
	if stats.TotalPageviews != 1 || len(stats.TopPages) != 1 || stats.TopPages[0].Name != "/qr" {
		t.Fatalf("expected QR-scoped stats, got %+v", stats)
	}
}
