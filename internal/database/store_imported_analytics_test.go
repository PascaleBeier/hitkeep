package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/importables"
)

func TestImportedSinkSkipsNativeTrafficOverlap(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "traffic-overlap.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	overlapDay := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := store.CreateHit(ctx, &api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: overlapDay,
		Path:      "/native",
	}); err != nil {
		t.Fatalf("create native hit: %v", err)
	}

	plan, err := store.BuildImportOverlapPlan(ctx, site.ID, overlapDay, overlapDay.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("build overlap plan: %v", err)
	}
	sink, err := NewImportedDataSinkWithOptions(ctx, store, site.ID, uuid.New(), ImportedDataSinkOptions{Overlap: plan})
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: overlapDay, Visitors: 2, Visits: 2, Pageviews: 7, SourceFile: "imported_visitors.csv"}); err != nil {
		t.Fatalf("put overlapped traffic: %v", err)
	}
	clearDay := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: clearDay, Visitors: 3, Visits: 3, Pageviews: 9, SourceFile: "imported_visitors.csv"}); err != nil {
		t.Fatalf("put clear traffic: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	var pageviews int
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(pageviews), 0) FROM imported_traffic_daily WHERE site_id = ?`, site.ID).Scan(&pageviews); err != nil {
		t.Fatalf("query imported pageviews: %v", err)
	}
	if pageviews != 9 {
		t.Fatalf("expected only non-overlapping imported pageviews, got %d", pageviews)
	}
}

func TestImportedSinkSkipsNativeEventOverlap(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "event-overlap.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	overlapDay := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := store.CreateEvent(ctx, &api.Event{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		Name:      "signup",
		Timestamp: overlapDay,
	}); err != nil {
		t.Fatalf("create native event: %v", err)
	}

	plan, err := store.BuildImportOverlapPlan(ctx, site.ID, overlapDay, overlapDay.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("build overlap plan: %v", err)
	}
	sink, err := NewImportedDataSinkWithOptions(ctx, store, site.ID, uuid.New(), ImportedDataSinkOptions{Overlap: plan})
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: overlapDay, EventName: "signup", Visitors: 2, Events: 4, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put overlapped event: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: overlapDay, EventName: "purchase", Visitors: 1, Events: 1, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put clear same-day event: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: overlapDay.AddDate(0, 0, 1), EventName: "signup", Visitors: 3, Events: 3, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put clear next-day event: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	var events int
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(events), 0) FROM imported_event_daily WHERE site_id = ?`, site.ID).Scan(&events); err != nil {
		t.Fatalf("query imported events: %v", err)
	}
	if events != 4 {
		t.Fatalf("expected non-overlapping imported event totals only, got %d", events)
	}
}

func TestEventGoalsIncludeImportedAggregateVisitors(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-goal.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.CreateGoal(ctx, &api.Goal{SiteID: site.ID, Name: "Signup", Type: "event", Value: "signup"}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: day, Visitors: 10, Visits: 10, Pageviews: 20, SourceFile: "imported_visitors.csv"}); err != nil {
		t.Fatalf("put traffic: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: day, EventName: "signup", Visitors: 3, Events: 8, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put event: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{SiteID: site.ID, Start: day, End: day.AddDate(0, 0, 7)})
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if len(stats.Goals) != 1 || stats.Goals[0].Conversions != 3 {
		t.Fatalf("expected imported visitors to count as event goal conversions, got %+v", stats.Goals)
	}
}

func TestImportedAggregatesIncludedWithComparisonRequest(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-comparison.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.CreateGoal(ctx, &api.Goal{SiteID: site.ID, Name: "Signup", Type: "event", Value: "signup"}); err != nil {
		t.Fatalf("create goal: %v", err)
	}

	day := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	putImportedComparisonRows(t, ctx, store, site.ID, day)
	stats := getComparisonDashboardStats(t, ctx, store, site.ID, day)
	assertImportedComparisonStats(t, stats)
}

func TestImportedSiteStatsIncludeGeoNetworkDimensions(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-geo-network.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	day := time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC)
	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: day, Visitors: 12, Visits: 12, Pageviews: 24, SourceFile: "datapoints.csv"}); err != nil {
		t.Fatalf("put traffic: %v", err)
	}
	for _, row := range []importables.DimensionRow{
		{Date: day, Dimension: "city", Name: "Dortmund", Visitors: 7, Visits: 7, Pageviews: 14, SourceFile: "datapoints.csv"},
		{Date: day, Dimension: "provider", Name: "Deutsche Telekom AG", Visitors: 6, Visits: 6, Pageviews: 12, SourceFile: "datapoints.csv"},
		{Date: day, Dimension: "asn", Name: "AS3320 Deutsche Telekom AG", Visitors: 5, Visits: 5, Pageviews: 10, SourceFile: "datapoints.csv"},
	} {
		if err := sink.PutDimension(ctx, row); err != nil {
			t.Fatalf("put dimension %s: %v", row.Dimension, err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{SiteID: site.ID, Start: day, End: day.AddDate(0, 0, 7)})
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	if !containsMetric(stats.TopCities, "Dortmund", 14) {
		t.Fatalf("expected imported city aggregate, got %+v", stats.TopCities)
	}
	if !containsMetric(stats.TopProviders, "Deutsche Telekom AG", 12) {
		t.Fatalf("expected imported provider aggregate, got %+v", stats.TopProviders)
	}
	if !containsMetric(stats.TopASNs, "AS3320 Deutsche Telekom AG", 10) {
		t.Fatalf("expected imported ASN aggregate, got %+v", stats.TopASNs)
	}
}

func putImportedComparisonRows(t *testing.T, ctx context.Context, store *Store, siteID uuid.UUID, day time.Time) {
	t.Helper()
	sink, err := NewImportedDataSink(ctx, store, siteID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	if err := sink.PutTraffic(ctx, importables.TrafficRow{Date: day, Visitors: 10, Visits: 10, Pageviews: 20, SourceFile: "datapoints.csv"}); err != nil {
		t.Fatalf("put traffic: %v", err)
	}
	if err := sink.PutDimension(ctx, importables.DimensionRow{Date: day, Dimension: "page", Name: "/pricing", Visitors: 3, Visits: 3, Pageviews: 6, SourceFile: "datapoints.csv"}); err != nil {
		t.Fatalf("put dimension: %v", err)
	}
	if err := sink.PutEvent(ctx, importables.EventRow{Date: day, EventName: "signup", Visitors: 4, Events: 7, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put event: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
}

func getComparisonDashboardStats(t *testing.T, ctx context.Context, store *Store, siteID uuid.UUID, day time.Time) *api.SiteStats {
	t.Helper()
	stats, err := store.GetSiteStats(ctx, api.AnalyticsParams{
		SiteID:       siteID,
		Start:        day.AddDate(0, 0, -1),
		End:          day.AddDate(0, 0, 6),
		CompareStart: day.AddDate(0, 0, -8),
		CompareEnd:   day.AddDate(0, 0, -1).Add(-time.Nanosecond),
	})
	if err != nil {
		t.Fatalf("get stats: %v", err)
	}
	return stats
}

func assertImportedComparisonStats(t *testing.T, stats *api.SiteStats) {
	t.Helper()
	if stats.TotalPageviews != 20 || stats.UniqueSessions != 10 {
		t.Fatalf("expected imported traffic with comparison, got pageviews=%d sessions=%d", stats.TotalPageviews, stats.UniqueSessions)
	}
	if !containsMetric(stats.TopPages, "/pricing", 6) {
		t.Fatalf("expected imported page dimension with comparison, got %+v", stats.TopPages)
	}
	if len(stats.Goals) != 1 || stats.Goals[0].Conversions != 4 {
		t.Fatalf("expected imported event goal conversions with comparison, got %+v", stats.Goals)
	}
	if stats.Comparison == nil {
		t.Fatal("expected comparison stats to still be returned")
	}
	if totalChartPageviews(stats.ChartData) != 20 {
		t.Fatalf("expected imported chart pageviews with comparison, got %+v", stats.ChartData)
	}
}

func totalChartPageviews(points []api.ChartDataPoint) int {
	var pageviews int
	for _, point := range points {
		pageviews += point.Pageviews
	}
	return pageviews
}

func TestEventAudienceReportsMissingImportedDimensions(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-audience.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := sink.PutEvent(ctx, importables.EventRow{Date: day, EventName: "signup", Path: "/pricing", Visitors: 3, Events: 8, SourceFile: "imported_custom_events.csv"}); err != nil {
		t.Fatalf("put event: %v", err)
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	audience, err := store.GetEventAudience(ctx, api.EventAudienceParams{SiteID: site.ID, Start: day, End: day.AddDate(0, 0, 7), EventName: "signup"})
	if err != nil {
		t.Fatalf("get audience: %v", err)
	}
	if len(audience.TopPages) != 1 || audience.TopPages[0].Name != "/pricing" || audience.TopPages[0].Value != 3 {
		t.Fatalf("expected imported path audience, got %+v", audience.TopPages)
	}
	if len(audience.ImportedExcluded) != 1 || audience.ImportedExcluded[0].Reason != "missing_event_dimension_relationships" {
		t.Fatalf("expected missing dimension limitation, got %+v", audience.ImportedExcluded)
	}
}

func TestEventAudienceIncludesImportedEventDimensions(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "imported-audience-dimensions.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	sink, err := NewImportedDataSink(ctx, store, site.ID, uuid.New())
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	day := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if err := sink.PutEvent(ctx, importables.EventRow{Date: day, EventName: "signup", Path: "/pricing", Visitors: 3, Events: 8, SourceFile: "imported_events.csv"}); err != nil {
		t.Fatalf("put event: %v", err)
	}
	for _, row := range []importables.EventDimensionRow{
		{Date: day, EventName: "signup", Dimension: "path", Name: "/pricing", Visitors: 3, Events: 8, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "referrer", Name: "Google", Visitors: 2, Events: 5, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "device", Name: "Desktop", Visitors: 2, Events: 5, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "country", Name: "Germany", Visitors: 1, Events: 3, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "city", Name: "Berlin", Visitors: 1, Events: 3, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "provider", Name: "Hetzner Online GmbH", Visitors: 1, Events: 3, SourceFile: "imported_event_dimensions.csv"},
		{Date: day, EventName: "signup", Dimension: "asn", Name: "AS24940 Hetzner Online GmbH", Visitors: 1, Events: 3, SourceFile: "imported_event_dimensions.csv"},
	} {
		if err := sink.PutEventDimension(ctx, row); err != nil {
			t.Fatalf("put event dimension %s: %v", row.Dimension, err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	audience, err := store.GetEventAudience(ctx, api.EventAudienceParams{SiteID: site.ID, Start: day, End: day.AddDate(0, 0, 7), EventName: "signup"})
	if err != nil {
		t.Fatalf("get audience: %v", err)
	}
	if len(audience.TopPages) != 1 || audience.TopPages[0].Name != "/pricing" || audience.TopPages[0].Value != 3 {
		t.Fatalf("expected imported path audience, got %+v", audience.TopPages)
	}
	if len(audience.TopReferrers) != 1 || audience.TopReferrers[0].Name != "Google" || audience.TopReferrers[0].Value != 2 {
		t.Fatalf("expected imported referrer audience, got %+v", audience.TopReferrers)
	}
	if len(audience.TopDevices) != 1 || audience.TopDevices[0].Name != "Desktop" || audience.TopDevices[0].Value != 2 {
		t.Fatalf("expected imported device audience, got %+v", audience.TopDevices)
	}
	if len(audience.TopCountries) != 1 || audience.TopCountries[0].Name != "Germany" || audience.TopCountries[0].Value != 1 {
		t.Fatalf("expected imported country audience, got %+v", audience.TopCountries)
	}
	if len(audience.TopCities) != 1 || audience.TopCities[0].Name != "Berlin" || audience.TopCities[0].Value != 1 {
		t.Fatalf("expected imported city audience, got %+v", audience.TopCities)
	}
	if len(audience.TopProviders) != 1 || audience.TopProviders[0].Name != "Hetzner Online GmbH" || audience.TopProviders[0].Value != 1 {
		t.Fatalf("expected imported provider audience, got %+v", audience.TopProviders)
	}
	if len(audience.TopASNs) != 1 || audience.TopASNs[0].Name != "AS24940 Hetzner Online GmbH" || audience.TopASNs[0].Value != 1 {
		t.Fatalf("expected imported ASN audience, got %+v", audience.TopASNs)
	}
	if len(audience.ImportedExcluded) != 0 {
		t.Fatalf("expected no missing-dimension limitation when aggregate dimensions exist, got %+v", audience.ImportedExcluded)
	}
}

func TestDeleteImportedDataForImportWithUnattributedProperties(t *testing.T) {
	store, userID := setupEventBreakdownStore(t)
	ctx := context.Background()
	site, err := store.CreateSite(ctx, userID, "import-delete-properties.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}

	importID := uuid.New()
	sink, err := NewImportedDataSink(ctx, store, site.ID, importID)
	if err != nil {
		t.Fatalf("new sink: %v", err)
	}
	dates := []time.Time{
		time.Date(2026, 4, 8, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 14, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
	}
	for _, date := range dates {
		if err := sink.PutEventProperty(ctx, importables.EventPropertyRow{
			Date:          date,
			EventName:     "outbound_click",
			PropertyKey:   "url",
			PropertyValue: "https://example.com",
			Visitors:      1,
			Events:        1,
			SourceFile:    "imported_custom_events.csv",
		}); err != nil {
			t.Fatalf("put attributed property: %v", err)
		}
		if err := sink.PutEventProperty(ctx, importables.EventPropertyRow{
			Date:          date,
			PropertyKey:   "url",
			PropertyValue: "https://example.com",
			Visitors:      1,
			Events:        1,
			SourceFile:    "imported_unattributed_properties.csv",
		}); err != nil {
			t.Fatalf("put unattributed property: %v", err)
		}
		if err := sink.PutEventDimension(ctx, importables.EventDimensionRow{Date: date, EventName: "outbound_click", Dimension: "url", Name: "https://example.com", Visitors: 1, Events: 1, SourceFile: "imported_event_dimensions.csv"}); err != nil {
			t.Fatalf("put event dimension: %v", err)
		}
	}
	if err := sink.Flush(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}

	var nullEventNames int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM imported_event_properties_daily WHERE site_id = ? AND event_name IS NULL`, site.ID).Scan(&nullEventNames); err != nil {
		t.Fatalf("query null imported property event names: %v", err)
	}
	if nullEventNames != 0 {
		t.Fatalf("expected unattributed properties to use non-null event name sentinel, got %d null rows", nullEventNames)
	}

	if _, err := store.DeleteImportedDataForImport(ctx, site.ID, importID); err != nil {
		t.Fatalf("delete imported data: %v", err)
	}
	var remaining int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM imported_event_properties_daily WHERE site_id = ?`, site.ID).Scan(&remaining); err != nil {
		t.Fatalf("query remaining imported properties after delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected imported property rows to be deleted, got %d", remaining)
	}
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM imported_event_dimensions_daily WHERE site_id = ?`, site.ID).Scan(&remaining); err != nil {
		t.Fatalf("query remaining imported dimensions after delete: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected imported event dimension rows to be deleted, got %d", remaining)
	}
}
