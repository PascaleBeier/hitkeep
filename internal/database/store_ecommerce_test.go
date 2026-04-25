package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestGetEcommerceSummaryAndBreakdowns(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "ecommerce@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "shop.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	firstSession := uuid.New()
	secondSession := uuid.New()
	firstDay := time.Date(2026, 3, 5, 10, 0, 0, 0, time.UTC)
	secondDay := firstDay.Add(24 * time.Hour)
	isUnique := true

	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:        site.ID,
		SessionID:     firstSession,
		PageID:        uuid.New(),
		Path:          "/pricing",
		Timestamp:     firstDay,
		Referrer:      new("https://www.google.com"),
		ViewportWidth: new(1440),
		CountryCode:   new("US"),
		UTMSource:     new("google"),
		UTMMedium:     new("cpc"),
		UTMCampaign:   new("spring-launch"),
		IsUnique:      &isUnique,
	}); err != nil {
		t.Fatalf("create first hit: %v", err)
	}

	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:        site.ID,
		SessionID:     secondSession,
		PageID:        uuid.New(),
		Path:          "/checkout",
		Timestamp:     secondDay,
		ViewportWidth: new(390),
		CountryCode:   new("DE"),
		UTMSource:     new("newsletter"),
		UTMMedium:     new("email"),
		UTMCampaign:   new("march-digest"),
		IsUnique:      &isUnique,
	}); err != nil {
		t.Fatalf("create second hit: %v", err)
	}

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: firstSession,
		Name:      "begin_checkout",
		Timestamp: firstDay.Add(15 * time.Minute),
		Properties: map[string]any{
			"items": []map[string]any{
				{"item_id": "pro-plan", "item_name": "Pro Plan", "quantity": 1, "price": 120.0},
			},
		},
	}); err != nil {
		t.Fatalf("create checkout event: %v", err)
	}

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: firstSession,
		Name:      "purchase",
		Timestamp: firstDay.Add(30 * time.Minute),
		Properties: map[string]any{
			"transaction_id": "ord_1001",
			"value":          120.0,
			"currency":       "USD",
			"coupon":         "SPRING25",
			"items": []map[string]any{
				{"item_id": "pro-plan", "item_name": "Pro Plan", "quantity": 1, "price": 120.0},
			},
		},
	}); err != nil {
		t.Fatalf("create purchase event: %v", err)
	}

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: secondSession,
		Name:      "checkout_started",
		Timestamp: secondDay.Add(20 * time.Minute),
		Properties: map[string]any{
			"items": []map[string]any{
				{"product_id": "starter-plan", "product_name": "Starter Plan", "quantity": 2, "price": 30.0},
			},
		},
	}); err != nil {
		t.Fatalf("create alias checkout event: %v", err)
	}

	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: secondSession,
		Name:      "order_completed",
		Timestamp: secondDay.Add(40 * time.Minute),
		Properties: map[string]any{
			"order_id": "ord_1002",
			"amount":   60.0,
			"currency": "USD",
			"items": []map[string]any{
				{"product_id": "starter-plan", "product_name": "Starter Plan", "quantity": 2, "price": 30.0},
			},
		},
	}); err != nil {
		t.Fatalf("create alias purchase event: %v", err)
	}

	params := api.EcommerceParams{
		SiteID: site.ID,
		Start:  firstDay.Add(-24 * time.Hour),
		End:    secondDay.Add(24 * time.Hour),
		Limit:  10,
	}

	summary, err := store.GetEcommerceSummary(ctx, params)
	if err != nil {
		t.Fatalf("get ecommerce summary: %v", err)
	}
	if summary.Orders != 2 {
		t.Fatalf("expected 2 orders, got %d", summary.Orders)
	}
	if summary.CheckoutStarts != 2 {
		t.Fatalf("expected 2 checkout starts, got %d", summary.CheckoutStarts)
	}
	if summary.Revenue != 180 {
		t.Fatalf("expected revenue 180, got %f", summary.Revenue)
	}
	if summary.Currency != "USD" {
		t.Fatalf("expected USD currency, got %q", summary.Currency)
	}

	googleSummary, err := store.GetEcommerceSummary(ctx, api.EcommerceParams{
		SiteID:  site.ID,
		Start:   params.Start,
		End:     params.End,
		Filters: []api.Filter{{Type: "utm_source", Value: "google"}},
	})
	if err != nil {
		t.Fatalf("get filtered summary: %v", err)
	}
	if googleSummary.Orders != 1 || googleSummary.Revenue != 120 {
		t.Fatalf("expected google filtered summary to be 1 order / 120 revenue, got %+v", googleSummary)
	}

	itemSummary, err := store.GetEcommerceSummary(ctx, api.EcommerceParams{
		SiteID:   site.ID,
		Start:    params.Start,
		End:      params.End,
		ItemID:   "starter-plan",
		ItemName: "",
	})
	if err != nil {
		t.Fatalf("get item filtered summary: %v", err)
	}
	if itemSummary.Orders != 1 || itemSummary.Revenue != 60 {
		t.Fatalf("expected starter-plan summary to be 1 order / 60 revenue, got %+v", itemSummary)
	}

	series, err := store.GetEcommerceTimeSeries(ctx, params)
	if err != nil {
		t.Fatalf("get ecommerce timeseries: %v", err)
	}
	if len(series) < 2 {
		t.Fatalf("expected at least 2 series points, got %d", len(series))
	}
	var sawFirstDay, sawSecondDay bool
	for _, point := range series {
		switch point.Time.Format("2006-01-02") {
		case "2026-03-05":
			sawFirstDay = true
			if point.Orders != 1 || point.Revenue != 120 {
				t.Fatalf("expected first day 1 order / 120 revenue, got %+v", point)
			}
		case "2026-03-06":
			sawSecondDay = true
			if point.Orders != 1 || point.Revenue != 60 {
				t.Fatalf("expected second day 1 order / 60 revenue, got %+v", point)
			}
		}
	}
	if !sawFirstDay || !sawSecondDay {
		t.Fatalf("expected both days in series, got %+v", series)
	}

	products, err := store.GetEcommerceTopProducts(ctx, params)
	if err != nil {
		t.Fatalf("get top products: %v", err)
	}
	if len(products) != 2 {
		t.Fatalf("expected 2 products, got %d", len(products))
	}
	if products[0].ItemID != "pro-plan" || products[0].Revenue != 120 {
		t.Fatalf("expected pro-plan to lead, got %+v", products[0])
	}
	if products[1].ItemID != "starter-plan" || products[1].Quantity != 2 {
		t.Fatalf("expected starter-plan quantity 2, got %+v", products[1])
	}

	sources, err := store.GetEcommerceSources(ctx, params)
	if err != nil {
		t.Fatalf("get ecommerce sources: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 source rows, got %d", len(sources))
	}
	if sources[0].UTMSource != "google" || sources[0].Revenue != 120 {
		t.Fatalf("expected google source to lead, got %+v", sources[0])
	}
}

func TestGetEcommerceSummaryWithoutPurchasesLeavesCurrencyEmpty(t *testing.T) {
	t.Parallel()

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	userID, err := store.CreateUser(ctx, "empty-ecommerce@test.dev", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "empty.shop.test")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	now := time.Date(2026, 3, 8, 10, 0, 0, 0, time.UTC)
	summary, err := store.GetEcommerceSummary(ctx, api.EcommerceParams{
		SiteID: site.ID,
		Start:  now.Add(-24 * time.Hour),
		End:    now,
	})
	if err != nil {
		t.Fatalf("get ecommerce summary: %v", err)
	}

	if summary.Currency != "" {
		t.Fatalf("expected empty currency for no ecommerce purchases, got %q", summary.Currency)
	}
	if summary.Revenue != 0 || summary.Orders != 0 || summary.CheckoutStarts != 0 {
		t.Fatalf("expected zeroed summary, got %+v", summary)
	}
}
