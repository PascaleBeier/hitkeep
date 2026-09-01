package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/hklog"
	"hitkeep/internal/api"
	"hitkeep/internal/assetstore"
	"hitkeep/internal/database"
)

type qrCampaignSeedFixture struct {
	name           string
	path           string
	utmSource      string
	utmMedium      string
	utmCampaign    string
	utmTerm        string
	utmContent     string
	customParams   map[string]string
	style          map[string]any
	followupPaths  []string
	referrers      []string
	openCount      int
	sessionCount   int
	conversionName string
	withAsset      bool
	withShare      bool
}

func seedQRCampaigns(ctx context.Context, sharedStore, analyticsStore *database.Store, siteID, userID uuid.UUID, domain string, numDays int, dataPath string, rng *mrand.Rand) (seedStats, error) {
	fixtures := qrCampaignSeedFixtures(domain)
	stats := seedStats{}
	var batch seedWriteBatch
	now := time.Now().UTC()
	daysBack := min(max(numDays, 7), 30)

	for fixtureIndex, fixture := range fixtures {
		qr, _, err := sharedStore.CreateQRCode(ctx, siteID, userID, api.QRCodeCreateRequest{
			Name:           fixture.name,
			DestinationURL: seedURLForPath(domain, fixture.path),
			UTMSource:      fixture.utmSource,
			UTMMedium:      fixture.utmMedium,
			UTMCampaign:    fixture.utmCampaign,
			UTMTerm:        fixture.utmTerm,
			UTMContent:     fixture.utmContent,
			CustomParams:   fixture.customParams,
			Style:          fixture.style,
		})
		if err != nil {
			return stats, fmt.Errorf("create QR campaign %q: %w", fixture.name, err)
		}
		stats.qrCodes++

		if fixture.withAsset {
			if err := seedQRCodeAsset(ctx, sharedStore, dataPath, siteID, qr.ID); err != nil {
				return stats, fmt.Errorf("seed QR campaign asset %q: %w", fixture.name, err)
			}
		}
		if fixture.withShare {
			if _, _, err := sharedStore.CreateQRCodeShareLink(ctx, siteID, qr.ID, userID); err != nil {
				return stats, fmt.Errorf("seed QR share link %q: %w", fixture.name, err)
			}
		}

		defaultRangeTS := defaultRangeQRCodeTimestamp(now, fixtureIndex, len(fixtures))
		open, err := seedQRCodeOpen(siteID, qr.ID, fixture, defaultRangeTS, rng)
		if err != nil {
			return stats, err
		}
		if err := analyticsStore.CreateQRCodeOpen(ctx, open); err != nil {
			return stats, fmt.Errorf("create default-range QR open for %q: %w", fixture.name, err)
		}
		stats.qrOpens++
		hits, events := seedQRCodeSession(&batch, siteID, qr.ID, domain, fixture, defaultRangeTS.Add(15*time.Second), 1, rng)
		stats.hits += hits
		stats.events += events
		stats.sessions++

		for i := 0; i < fixture.openCount; i++ {
			ts := randomQRCodeTimestamp(rng, now, daysBack)
			open, err := seedQRCodeOpen(siteID, qr.ID, fixture, ts, rng)
			if err != nil {
				return stats, err
			}
			if err := analyticsStore.CreateQRCodeOpen(ctx, open); err != nil {
				return stats, fmt.Errorf("create QR open for %q: %w", fixture.name, err)
			}
			stats.qrOpens++

			if i < fixture.sessionCount {
				hits, events := seedQRCodeSession(&batch, siteID, qr.ID, domain, fixture, ts.Add(15*time.Second), i, rng)
				stats.hits += hits
				stats.events += events
				stats.sessions++
			}
		}
	}

	if err := batch.flush(ctx, analyticsStore); err != nil {
		return stats, fmt.Errorf("flush QR campaign analytics: %w", err)
	}

	hklog.LoggerFromContext(ctx).Info("QR campaigns seeded", "codes", stats.qrCodes, "opens", stats.qrOpens, "attributed_hits", stats.hits, "attributed_sessions", stats.sessions)
	return stats, nil
}

func defaultRangeQRCodeTimestamp(now time.Time, index, total int) time.Time {
	now = now.UTC()
	dayStart := now.Truncate(24 * time.Hour)
	if ts := now.Add(-time.Duration(index+1) * time.Minute); ts.After(dayStart) {
		return ts
	}

	elapsed := now.Sub(dayStart)
	if elapsed <= 0 {
		return dayStart
	}
	denominator := max(total+1, 2)
	offset := time.Duration(index+1) * elapsed / time.Duration(denominator)
	return dayStart.Add(offset)
}

func qrCampaignSeedFixtures(domain string) []qrCampaignSeedFixture {
	_ = domain
	return []qrCampaignSeedFixture{
		{
			name:        "Conference booth poster",
			path:        "/pricing",
			utmSource:   "conference",
			utmMedium:   "qr",
			utmCampaign: "berlin-analytics-summit",
			utmContent:  "booth-poster",
			customParams: map[string]string{
				"placement": "booth-wall",
				"segment":   "enterprise",
			},
			style: map[string]any{
				"foreground":   "#0f172a",
				"background":   "#f8fafc",
				"dots":         "rounded",
				"corners":      "extra-rounded",
				"image_margin": 6,
			},
			followupPaths:  []string{"/pricing", "/signup", "/docs/getting-started", "/contact"},
			referrers:      []string{"https://events.example.com/berlin-analytics-summit", "https://scan.example.com/booth"},
			openCount:      58,
			sessionCount:   36,
			conversionName: "demo_requested",
			withAsset:      true,
			withShare:      true,
		},
		{
			name:        "Partner leave-behind card",
			path:        "/contact",
			utmSource:   "partner",
			utmMedium:   "qr",
			utmCampaign: "agency-leave-behind",
			utmContent:  "business-card",
			customParams: map[string]string{
				"partner": "studio-north",
				"region":  "dach",
			},
			style: map[string]any{
				"foreground":   "#064e3b",
				"background":   "#ffffff",
				"dots":         "classy-rounded",
				"corners":      "extra-rounded",
				"image_margin": 5,
			},
			followupPaths:  []string{"/contact", "/features", "/pricing", "/signup"},
			referrers:      []string{"https://partners.example.com/studio-north", "https://scan.example.com/card"},
			openCount:      42,
			sessionCount:   25,
			conversionName: "trial_started",
			withAsset:      false,
			withShare:      false,
		},
		{
			name:        "Retail window flyer",
			path:        "/signup",
			utmSource:   "print",
			utmMedium:   "qr",
			utmCampaign: "retail-window-spring",
			utmContent:  "window-flyer",
			customParams: map[string]string{
				"placement": "window",
				"city":      "berlin",
			},
			style: map[string]any{
				"foreground":   "#7c2d12",
				"background":   "#fff7ed",
				"dots":         "dots",
				"corners":      "dot",
				"image_margin": 7,
			},
			followupPaths:  []string{"/signup", "/pricing", "/features", "/docs/configuration"},
			referrers:      []string{"https://scan.example.com/window", "https://retail.example.com/flyer"},
			openCount:      31,
			sessionCount:   18,
			conversionName: "newsletter_signup",
			withAsset:      true,
			withShare:      false,
		},
	}
}

func seedQRCodeAsset(ctx context.Context, store *database.Store, dataPath string, siteID, qrID uuid.UUID) error {
	data, err := base64.StdEncoding.DecodeString(seedQRLogoPNGBase64)
	if err != nil {
		return err
	}
	checksum := sha256.Sum256(data)
	checksumHex := hex.EncodeToString(checksum[:])
	storageKey, err := assetstore.New(dataPath).PutQRCodeAsset(siteID, qrID, checksumHex, "hitkeep-demo-mark.png", "image/png", data)
	if err != nil {
		return err
	}
	_, err = store.UpsertQRCodeAsset(ctx, api.QRCodeAsset{
		QRCodeID:    qrID,
		SiteID:      siteID,
		Filename:    "hitkeep-demo-mark.png",
		ContentType: "image/png",
		ByteSize:    int64(len(data)),
		Width:       1,
		Height:      1,
		Checksum:    "sha256:" + checksumHex,
		StorageKey:  storageKey,
	})
	return err
}

func seedQRCodeOpen(siteID, qrID uuid.UUID, fixture qrCampaignSeedFixture, ts time.Time, rng *mrand.Rand) (*api.QRCodeOpen, error) {
	uaEntry := pickWeighted(rng, userAgents)
	country := pickWeighted(rng, countries)
	region, city, provider, asn, asnOrg := seedGeoNetworkMetadata(country, rng)
	var referrer *string
	if len(fixture.referrers) > 0 && rng.Float64() < 0.72 {
		referrer = new(fixture.referrers[rng.Intn(len(fixture.referrers))])
	}
	return &api.QRCodeOpen{
		SiteID:      siteID,
		QRCodeID:    qrID,
		Timestamp:   ts,
		Referrer:    referrer,
		UserAgent:   &uaEntry.ua,
		CountryCode: country,
		Region:      region,
		City:        city,
		Provider:    provider,
		ASN:         asn,
		ASNOrg:      asnOrg,
	}, nil
}

func seedQRCodeSession(batch *seedWriteBatch, siteID, qrID uuid.UUID, domain string, fixture qrCampaignSeedFixture, sessionStart time.Time, sessionIndex int, rng *mrand.Rand) (int, int) {
	sessionID := uuid.New()
	uaEntry := pickWeighted(rng, userAgents)
	country := pickWeighted(rng, countries)
	region, city, provider, asn, asnOrg := seedGeoNetworkMetadata(country, rng)
	lang := pickWeighted(rng, languages)
	vw, vh, sw, sh := pickViewport(rng, uaEntry.kind)
	hostname := seedHostname(domain)
	referrer := seedQRCodeHitReferrer(fixture, rng)
	sessionLen := 2 + rng.Intn(3)

	for i := range sessionLen {
		path := fixture.followupPaths[min(i, len(fixture.followupPaths)-1)]
		ts := sessionStart.Add(time.Duration(i*80+rng.Intn(80)) * time.Second)
		isUnique := i == 0
		hit := &api.Hit{
			SiteID:         siteID,
			SessionID:      sessionID,
			PageID:         uuid.New(),
			Timestamp:      ts,
			Path:           path,
			Hostname:       &hostname,
			UserAgent:      new(uaEntry.ua),
			CountryCode:    country,
			Region:         region,
			City:           city,
			Provider:       provider,
			ASN:            asn,
			ASNOrg:         asnOrg,
			Language:       lang,
			ViewportWidth:  &vw,
			ViewportHeight: &vh,
			ScreenWidth:    &sw,
			ScreenHeight:   &sh,
			UTMSource:      new(fixture.utmSource),
			UTMMedium:      new(fixture.utmMedium),
			UTMCampaign:    new(fixture.utmCampaign),
			QRCodeID:       &qrID,
			IsUnique:       &isUnique,
		}
		if fixture.utmTerm != "" {
			hit.UTMTerm = new(fixture.utmTerm)
		}
		if fixture.utmContent != "" {
			hit.UTMContent = new(fixture.utmContent)
		}
		if i == 0 {
			hit.Referrer = referrer
		}
		batch.addHit(hit)
	}

	events := seedQRCodeSessionEvents(batch, siteID, sessionID, fixture, sessionStart.Add(time.Duration(sessionLen*90)*time.Second), sessionIndex, rng)
	return sessionLen, events
}

func seedQRCodeSessionEvents(batch *seedWriteBatch, siteID, sessionID uuid.UUID, fixture qrCampaignSeedFixture, ts time.Time, sessionIndex int, rng *mrand.Rand) int {
	count := 0
	if fixture.conversionName != "" {
		batch.addEvent(&api.Event{
			SiteID:    siteID,
			SessionID: sessionID,
			Name:      fixture.conversionName,
			Properties: map[string]any{
				"source":       "qr",
				"utm_campaign": fixture.utmCampaign,
				"asset":        fixture.utmContent,
			},
			Timestamp: ts,
		})
		count++
	}

	if sessionIndex%4 != 0 {
		return count
	}

	product := pickWeighted(rng, ecommerceProducts)
	item := map[string]any{
		"item_id":       product.itemID,
		"item_name":     product.itemName,
		"item_category": product.category,
		"price":         product.price,
		"quantity":      1,
	}
	currency := "USD"
	value := float64(product.price)
	transactionID := fmt.Sprintf("qr_ord_%s", uuid.NewString()[:10])
	batch.addEvent(&api.Event{
		SiteID:    siteID,
		SessionID: sessionID,
		Name:      "begin_checkout",
		Properties: map[string]any{
			"checkout_id": fmt.Sprintf("qr_chk_%s", uuid.NewString()[:10]),
			"value":       value,
			"currency":    currency,
			"items_count": 1,
			"items":       []map[string]any{item},
		},
		Timestamp: ts.Add(45 * time.Second),
	})
	batch.addEvent(&api.Event{
		SiteID:    siteID,
		SessionID: sessionID,
		Name:      "purchase",
		Properties: map[string]any{
			"transaction_id": transactionID,
			"order_id":       transactionID,
			"value":          value,
			"amount":         value,
			"currency":       currency,
			"items_count":    1,
			"items":          []map[string]any{item},
		},
		Timestamp: ts.Add(2 * time.Minute),
	})
	count += 2
	return count
}

func seedQRCodeHitReferrer(fixture qrCampaignSeedFixture, rng *mrand.Rand) *string {
	if len(fixture.referrers) == 0 || rng.Float64() < 0.35 {
		return nil
	}
	return new(fixture.referrers[rng.Intn(len(fixture.referrers))])
}

func randomQRCodeTimestamp(rng *mrand.Rand, now time.Time, daysBack int) time.Time {
	day := now.AddDate(0, 0, -rng.Intn(max(daysBack, 1))).Truncate(24 * time.Hour)
	ts := randomTimeInDay(rng, day)
	if ts.After(now.Add(-10 * time.Minute)) {
		return now.Add(-time.Duration(10+rng.Intn(600)) * time.Minute)
	}
	return ts
}

func seedURLForPath(domain, path string) string {
	base := "https://" + seedHostname(domain)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

func seedHostname(domain string) string {
	host := strings.TrimSpace(domain)
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")
	host = strings.Trim(host, "/")
	if idx := strings.IndexByte(host, '/'); idx >= 0 {
		host = host[:idx]
	}
	if host == "" {
		return "acme-analytics.io"
	}
	return host
}

const seedQRLogoPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC"
