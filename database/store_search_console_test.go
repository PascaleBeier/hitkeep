package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/api"
	tenantmigrations "hitkeep/database/migrations/tenant"
)

func TestSearchConsoleFactsTenantMigration(t *testing.T) {
	store := newSearchConsoleTenantTestStore(t)

	expectedColumns := map[string]bool{
		"site_id":          false,
		"property_uri":     false,
		"date":             false,
		"query":            false,
		"page":             false,
		"country":          false,
		"device":           false,
		"clicks":           false,
		"impressions":      false,
		"ctr":              false,
		"position":         false,
		"aggregation_type": false,
		"data_state":       false,
		"imported_at":      false,
	}

	rows, err := store.DB().QueryContext(context.Background(), `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'search_console_facts'
	`)
	if err != nil {
		t.Fatalf("list Search Console facts columns: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var column string
		if err := rows.Scan(&column); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if _, ok := expectedColumns[column]; ok {
			expectedColumns[column] = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read columns: %v", err)
	}
	for column, found := range expectedColumns {
		if !found {
			t.Fatalf("expected search_console_facts column %q", column)
		}
	}
	var indexes, keyConstraints int
	if err := store.DB().QueryRowContext(context.Background(), `
		SELECT count(*) FROM duckdb_indexes() WHERE table_name = 'search_console_facts'
	`).Scan(&indexes); err != nil {
		t.Fatalf("count Search Console fact indexes: %v", err)
	}
	if err := store.DB().QueryRowContext(context.Background(), `
		SELECT count(*) FROM duckdb_constraints()
		WHERE table_name = 'search_console_facts' AND constraint_type IN ('PRIMARY KEY', 'UNIQUE', 'FOREIGN KEY')
	`).Scan(&keyConstraints); err != nil {
		t.Fatalf("count Search Console fact key constraints: %v", err)
	}
	if indexes != 0 || keyConstraints != 0 {
		t.Fatalf("expected Search Console facts without ART indexes, got indexes=%d constraints=%d", indexes, keyConstraints)
	}
}

func TestReplaceSearchConsoleFactsIsIdempotentAndDropsStaleRows(t *testing.T) {
	ctx := context.Background()
	store := newSearchConsoleTenantTestStore(t)
	siteID := uuid.New()
	rowDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	input := SearchConsoleFactInput{
		SiteID:          siteID,
		PropertyURI:     "sc-domain:example.com",
		Date:            rowDate,
		Query:           "hitkeep analytics",
		Page:            "https://example.com/",
		Country:         "USA",
		Device:          "DESKTOP",
		Clicks:          3,
		Impressions:     30,
		CTR:             0.1,
		Position:        4.2,
		AggregationType: "byPage",
		DataState:       "final",
		ImportedAt:      rowDate.Add(48 * time.Hour),
	}
	scope := SearchConsoleFactScope{SiteID: siteID, PropertyURI: input.PropertyURI, StartDate: rowDate, EndDate: rowDate, DataState: "final"}
	stale := input
	stale.Query = "stale query"
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, []SearchConsoleFactInput{input, stale}); err != nil {
		t.Fatalf("replace first facts: %v", err)
	}
	input.Clicks = 5
	input.Impressions = 50
	input.CTR = 0.2
	input.Position = 3.8
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, []SearchConsoleFactInput{input, input}); err != nil {
		t.Fatalf("replace second facts: %v", err)
	}

	var count, clicks, impressions int
	var ctr, position float64
	if err := store.DB().QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(MAX(clicks), 0), COALESCE(MAX(impressions), 0), COALESCE(MAX(ctr), 0), COALESCE(MAX(position), 0)
		FROM search_console_facts
		WHERE site_id = ?
	`, siteID).Scan(&count, &clicks, &impressions, &ctr, &position); err != nil {
		t.Fatalf("query facts: %v", err)
	}
	if count != 1 || clicks != 5 || impressions != 50 || ctr != 0.2 || position != 3.8 {
		t.Fatalf("expected one updated fact, got count=%d clicks=%d impressions=%d ctr=%f position=%f", count, clicks, impressions, ctr, position)
	}
}

func TestReplaceSearchConsoleFactsKeepsDistinctAggregationTypes(t *testing.T) {
	ctx := context.Background()
	store := newSearchConsoleTenantTestStore(t)
	siteID := uuid.New()
	rowDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	base := SearchConsoleFactInput{
		SiteID:      siteID,
		PropertyURI: "sc-domain:example.com",
		Date:        rowDate,
		Query:       "hitkeep analytics",
		Page:        "https://example.com/",
		Country:     "USA",
		Device:      "DESKTOP",
		DataState:   "final",
		ImportedAt:  rowDate.Add(48 * time.Hour),
	}
	byProperty := base
	byProperty.Clicks = 3
	byProperty.AggregationType = "byProperty"
	byPage := base
	byPage.Clicks = 7
	byPage.AggregationType = "byPage"
	if err := store.ReplaceSearchConsoleFacts(ctx, SearchConsoleFactScope{
		SiteID: siteID, PropertyURI: base.PropertyURI, StartDate: rowDate, EndDate: rowDate, DataState: "final",
	}, []SearchConsoleFactInput{byProperty, byPage}); err != nil {
		t.Fatalf("replace facts: %v", err)
	}

	var count, clicks int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(clicks), 0)
		FROM search_console_facts
		WHERE site_id = ?
	`, siteID).Scan(&count, &clicks); err != nil {
		t.Fatalf("query facts: %v", err)
	}
	if count != 2 || clicks != 10 {
		t.Fatalf("expected two aggregate-grain facts with 10 clicks, got count=%d clicks=%d", count, clicks)
	}
}

func TestReplaceSearchConsoleFactsSupportsIndexedCompatibilitySchema(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect compatibility store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, migration := range []string{"0008_create_search_console_facts.sql", "0017_create_search_console_totals_days.sql"} {
		createSQL, err := tenantmigrations.Fs.ReadFile(migration)
		if err != nil {
			t.Fatalf("read compatibility schema %s: %v", migration, err)
		}
		if _, err := store.DB().ExecContext(ctx, string(createSQL)); err != nil {
			t.Fatalf("create compatibility schema %s: %v", migration, err)
		}
	}

	siteID := uuid.New()
	rowDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	scope := SearchConsoleFactScope{
		SiteID: siteID, PropertyURI: "sc-domain:compat.example", StartDate: rowDate, EndDate: rowDate, DataState: "final",
	}
	fact := SearchConsoleFactInput{
		SiteID: siteID, PropertyURI: scope.PropertyURI, Date: rowDate, Query: "compatibility",
		Country: "US", Device: "desktop", Clicks: 3, AggregationType: "byProperty", DataState: "final",
	}
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, []SearchConsoleFactInput{fact}); err != nil {
		t.Fatalf("replace through indexed compatibility schema: %v", err)
	}
	fact.Clicks = 8
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, []SearchConsoleFactInput{fact}); err != nil {
		t.Fatalf("repeat replacement through indexed compatibility schema: %v", err)
	}

	var count, clicks int
	var country, device string
	if err := store.DB().QueryRowContext(ctx, `
		SELECT count(*), max(clicks), max(country), max(device)
		FROM search_console_facts WHERE site_id = ?
	`, siteID).Scan(&count, &clicks, &country, &device); err != nil {
		t.Fatalf("read compatibility facts: %v", err)
	}
	if count != 1 || clicks != 8 || country != "USA" || device != "DESKTOP" {
		t.Fatalf("unexpected compatibility replacement: count=%d clicks=%d country=%q device=%q", count, clicks, country, device)
	}
}

func TestReplaceSearchConsoleFactsValidatesBeforeDeleteAndKeepsOtherScopes(t *testing.T) {
	ctx := context.Background()
	store := newSearchConsoleTenantTestStore(t)
	siteID := uuid.New()
	otherSiteID := uuid.New()
	rowDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	base := SearchConsoleFactInput{
		SiteID: siteID, PropertyURI: "sc-domain:example.com", Date: rowDate, Query: "kept",
		Page: "https://example.com/", Country: "USA", Device: "DESKTOP", Clicks: 3, DataState: "final",
	}
	other := base
	other.SiteID = otherSiteID
	other.PropertyURI = "sc-domain:other.example"
	if err := store.ReplaceSearchConsoleFacts(ctx, SearchConsoleFactScope{
		SiteID: siteID, PropertyURI: base.PropertyURI, StartDate: rowDate, EndDate: rowDate, DataState: "final",
	}, []SearchConsoleFactInput{base}); err != nil {
		t.Fatalf("seed scoped fact: %v", err)
	}
	if err := store.ReplaceSearchConsoleFacts(ctx, SearchConsoleFactScope{
		SiteID: otherSiteID, PropertyURI: other.PropertyURI, StartDate: rowDate, EndDate: rowDate, DataState: "final",
	}, []SearchConsoleFactInput{other}); err != nil {
		t.Fatalf("seed other fact: %v", err)
	}

	invalid := base
	invalid.Date = rowDate.AddDate(0, 0, 1)
	if err := store.ReplaceSearchConsoleFacts(ctx, SearchConsoleFactScope{
		SiteID: siteID, PropertyURI: base.PropertyURI, StartDate: rowDate, EndDate: rowDate, DataState: "final",
	}, []SearchConsoleFactInput{invalid}); err == nil {
		t.Fatal("expected out-of-scope fact to fail")
	}

	var kept, otherKept int
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM search_console_facts WHERE site_id = ?", siteID).Scan(&kept); err != nil {
		t.Fatalf("count preserved scoped fact: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM search_console_facts WHERE site_id = ?", otherSiteID).Scan(&otherKept); err != nil {
		t.Fatalf("count preserved other fact: %v", err)
	}
	if kept != 1 || otherKept != 1 {
		t.Fatalf("expected validation to preserve both scopes, got %d and %d", kept, otherKept)
	}
}

func TestReplaceSearchConsoleFactsRollsBackDeleteWhenAppendFails(t *testing.T) {
	ctx := context.Background()
	store := newSearchConsoleTenantTestStore(t)
	siteID := uuid.New()
	rowDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	scope := SearchConsoleFactScope{
		SiteID: siteID, PropertyURI: "sc-domain:example.com", StartDate: rowDate, EndDate: rowDate, DataState: "final",
		TotalsComplete: true,
	}
	original := SearchConsoleFactInput{
		SiteID: siteID, PropertyURI: scope.PropertyURI, Date: rowDate, Query: "kept after rollback",
		Clicks: 3, AggregationType: "byProperty", DataState: "final",
	}
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, []SearchConsoleFactInput{original}); err != nil {
		t.Fatalf("seed fact before rollback test: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, "ALTER TABLE search_console_facts DROP COLUMN position"); err != nil {
		t.Fatalf("make appender schema invalid: %v", err)
	}

	replacement := original
	replacement.Query = "must not commit"
	replacement.Clicks = 9
	scope.EndDate = rowDate.AddDate(0, 0, 1)
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, []SearchConsoleFactInput{replacement}); err == nil {
		t.Fatal("expected appender setup failure")
	}

	var query string
	var clicks int
	if err := store.DB().QueryRowContext(ctx, `
		SELECT query, clicks FROM search_console_facts
		WHERE site_id = ? AND property_uri = ? AND date = ?
	`, siteID, scope.PropertyURI, rowDate).Scan(&query, &clicks); err != nil {
		t.Fatalf("read fact after failed replacement: %v", err)
	}
	if query != original.Query || clicks != original.Clicks {
		t.Fatalf("expected original fact after rollback, got query=%q clicks=%d", query, clicks)
	}
	days, err := store.ListSearchConsoleAudienceTotalsDays(ctx, siteID, scope.PropertyURI, scope.StartDate, scope.EndDate)
	if err != nil || len(days) != 1 || !days[0].Equal(rowDate) {
		t.Fatalf("failed replacement changed completion checkpoints: %v, err=%v", days, err)
	}
}

func TestDropSearchConsoleFactIndexesMigrationPreservesDataAndConstraints(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	createSQL, err := tenantmigrations.Fs.ReadFile("0008_create_search_console_facts.sql")
	if err != nil {
		t.Fatalf("read original Search Console migration: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, string(createSQL)); err != nil {
		t.Fatalf("create indexed Search Console facts: %v", err)
	}
	siteID := uuid.New()
	rowDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO search_console_facts (site_id, property_uri, date, query, clicks, imported_at)
		VALUES (?, 'sc-domain:example.com', ?, 'preserved', 9, now())
	`, siteID, rowDate); err != nil {
		t.Fatalf("seed indexed Search Console fact: %v", err)
	}

	migrationSQL, err := tenantmigrations.Fs.ReadFile("0016_drop_search_console_fact_art_indexes.sql")
	if err != nil {
		t.Fatalf("read Search Console index-drop migration: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, string(migrationSQL)); err != nil {
		t.Fatalf("apply Search Console index-drop migration: %v", err)
	}

	var rows, clicks int
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*), sum(clicks) FROM search_console_facts WHERE site_id = ?", siteID).Scan(&rows, &clicks); err != nil {
		t.Fatalf("read preserved Search Console fact: %v", err)
	}
	if rows != 1 || clicks != 9 {
		t.Fatalf("expected preserved Search Console fact, got rows=%d clicks=%d", rows, clicks)
	}
	var indexes, keyConstraints int
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM duckdb_indexes() WHERE table_name = 'search_console_facts'").Scan(&indexes); err != nil {
		t.Fatalf("count rebuilt indexes: %v", err)
	}
	if err := store.DB().QueryRowContext(ctx, `
		SELECT count(*) FROM duckdb_constraints()
		WHERE table_name = 'search_console_facts' AND constraint_type IN ('PRIMARY KEY', 'UNIQUE', 'FOREIGN KEY')
	`).Scan(&keyConstraints); err != nil {
		t.Fatalf("count rebuilt key constraints: %v", err)
	}
	if indexes != 0 || keyConstraints != 0 {
		t.Fatalf("expected rebuilt table without ART indexes, got indexes=%d constraints=%d", indexes, keyConstraints)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO search_console_facts (site_id, property_uri, date, imported_at)
		VALUES (?, 'sc-domain:defaults.example', ?, now())
	`, uuid.New(), rowDate); err != nil {
		t.Fatalf("expected rebuilt defaults to remain usable: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO search_console_facts (site_id, property_uri, date, query, imported_at)
		VALUES (?, 'sc-domain:null.example', ?, NULL, now())
	`, uuid.New(), rowDate); err == nil {
		t.Fatal("expected rebuilt query NOT NULL constraint")
	}
	for _, fileName := range []string{
		"2026_07_30_010000_drop_search_console_fact_art_indexes.sql",
		"0016_drop_search_console_fact_art_indexes.sql",
	} {
		if !migrationNeedsNativeReopen(fileName) {
			t.Fatalf("expected %s to use guarded native reopen", fileName)
		}
	}
}

func TestReplaceSearchConsoleFactsMaximumDailyPageUnderLowMemoryLimit(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:", WithMemoryLimit("1GiB"), WithThreads(1))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect low-memory store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.MigrateTenant(ctx); err != nil {
		t.Fatalf("migrate low-memory tenant store: %v", err)
	}

	const rowCount = 50_000
	siteID := uuid.New()
	rowDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	propertyURI := "sc-domain:large-history.example"
	queryPadding := strings.Repeat("query-", 20)
	pagePadding := strings.Repeat("segment/", 20)
	inputs := make([]SearchConsoleFactInput, 0, rowCount)
	for i := range rowCount {
		inputs = append(inputs, SearchConsoleFactInput{
			SiteID: siteID, PropertyURI: propertyURI, Date: rowDate,
			Query: fmt.Sprintf("%s%d", queryPadding, i), Page: fmt.Sprintf("https://large-history.example/%s%d", pagePadding, i),
			Country: "USA", Device: "DESKTOP", Clicks: i % 20, Impressions: 100 + i%100,
			CTR: 0.1, Position: 3.5, AggregationType: "byPage", DataState: "final", ImportedAt: rowDate.Add(48 * time.Hour),
		})
	}
	if err := store.ReplaceSearchConsoleFacts(ctx, SearchConsoleFactScope{
		SiteID: siteID, PropertyURI: propertyURI, StartDate: rowDate, EndDate: rowDate, DataState: "final",
	}, inputs); err != nil {
		t.Fatalf("replace maximum daily Search Console page: %v", err)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT count(*) FROM search_console_facts WHERE site_id = ?", siteID).Scan(&count); err != nil {
		t.Fatalf("count low-memory Search Console facts: %v", err)
	}
	if count != rowCount {
		t.Fatalf("expected %d low-memory facts, got %d", rowCount, count)
	}
}

func TestSearchConsoleReportsUseTotalsWithoutDoubleCountingQueryDetails(t *testing.T) {
	ctx := context.Background()
	store := newSearchConsoleTenantTestStore(t)
	siteID := uuid.New()
	date := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	legacyDate := date.AddDate(0, 0, -1)
	property := "sc-domain:example.com"
	detail := SearchConsoleFactInput{
		SiteID: siteID, PropertyURI: property, Date: date, Query: "visible query",
		Page: "https://example.com/landing", Country: "USA", Device: "DESKTOP",
		Clicks: 2, Impressions: 40, Position: 20, DataState: "final",
	}
	pageTotal := detail
	pageTotal.Query, pageTotal.Country, pageTotal.Device = "", "", ""
	pageTotal.Clicks, pageTotal.Impressions, pageTotal.Position = 6, 100, 12
	audienceTotal := detail
	audienceTotal.Query, audienceTotal.Page = "", ""
	audienceTotal.Clicks, audienceTotal.Impressions, audienceTotal.Position = 6, 100, 12
	legacy := detail
	legacy.Date, legacy.Query, legacy.Clicks, legacy.Impressions = legacyDate, "legacy query", 3, 50
	scope := SearchConsoleFactScope{SiteID: siteID, PropertyURI: property, StartDate: legacyDate, EndDate: date, DataState: "final"}
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, []SearchConsoleFactInput{detail, pageTotal, audienceTotal, legacy}); err != nil {
		t.Fatalf("seed mixed totals and legacy data: %v", err)
	}
	// Totals for another site, property or data state must not hide this
	// site's legacy rows.
	otherSite, otherProperty, otherState := audienceTotal, audienceTotal, audienceTotal
	otherSite.SiteID = uuid.New()
	otherProperty.PropertyURI = "sc-domain:other.example"
	otherState.DataState = "all"
	for _, other := range []SearchConsoleFactInput{otherSite, otherProperty, otherState} {
		other.Date = legacyDate
		if err := store.ReplaceSearchConsoleFacts(ctx, SearchConsoleFactScope{
			SiteID: other.SiteID, PropertyURI: other.PropertyURI, StartDate: legacyDate, EndDate: legacyDate, DataState: other.DataState,
		}, []SearchConsoleFactInput{other}); err != nil {
			t.Fatalf("seed unrelated totals: %v", err)
		}
	}

	totalsDays, err := store.ListSearchConsoleAudienceTotalsDays(ctx, siteID, property, legacyDate, date)
	if err != nil {
		t.Fatalf("list audience totals days: %v", err)
	}
	if len(totalsDays) != 1 || !totalsDays[0].Equal(date) {
		t.Fatalf("expected only this site's totals day %s, got %v", date, totalsDays)
	}

	params := api.SearchConsoleReportParams{SiteID: siteID, PropertyURI: property, Start: legacyDate, End: date}
	overview, err := store.GetSearchConsoleOverview(ctx, params)
	if err != nil {
		t.Fatalf("get overview: %v", err)
	}
	if overview.Clicks != 9 || overview.Impressions != 150 || overview.CTR != 0.06 || overview.AveragePosition != (12.0*100+20*50)/150 {
		t.Fatalf("expected totals plus legacy fallback, got %+v", overview)
	}
	series, err := store.GetSearchConsoleSeries(ctx, params)
	if err != nil {
		t.Fatalf("get series: %v", err)
	}
	if len(series.Series) != 2 || series.Series[0].Clicks != 3 || series.Series[1].Clicks != 6 || series.Series[1].Impressions != 100 {
		t.Fatalf("expected daily totals without double counting, got %+v", series)
	}
	for _, dimension := range []string{"page", "country", "device"} {
		result, err := store.GetSearchConsoleDimension(ctx, params, dimension)
		if err != nil {
			t.Fatalf("get %s report: %v", dimension, err)
		}
		if len(result.Rows) != 1 || result.Rows[0].Clicks != 9 || result.Rows[0].Impressions != 150 {
			t.Fatalf("expected %s totals without double counting, got %+v", dimension, result)
		}
	}
	queries, err := store.GetSearchConsoleDimension(ctx, params, "query")
	if err != nil {
		t.Fatalf("get query report: %v", err)
	}
	if len(queries.Rows) != 2 || queries.Rows[0].Value != "legacy query" || queries.Rows[0].Clicks != 3 ||
		queries.Rows[1].Value != "visible query" || queries.Rows[1].Clicks != 2 || queries.Rows[1].Impressions != 40 {
		t.Fatalf("expected only named query rows, got %+v", queries)
	}

	day := params
	day.Start = date
	filters := []struct {
		name  string
		apply func(*api.SearchConsoleReportParams)
	}{
		{"page", func(p *api.SearchConsoleReportParams) { p.Page = detail.Page }},
		{"path", func(p *api.SearchConsoleReportParams) { p.Path = "/landing" }},
		{"country", func(p *api.SearchConsoleReportParams) { p.Country = "us" }},
		{"device", func(p *api.SearchConsoleReportParams) { p.Device = "desktop" }},
	}
	for _, filter := range filters {
		filtered := day
		filter.apply(&filtered)
		result, err := store.GetSearchConsoleOverview(ctx, filtered)
		if err != nil || result.Clicks != 6 || result.Impressions != 100 {
			t.Fatalf("%s filter lost anonymized clicks: result=%+v err=%v", filter.name, result, err)
		}
	}
	// Neither totals shape groups by page and country together, so that
	// combination can only use the query rows.
	combined := day
	combined.Page, combined.Country = detail.Page, "us"
	if result, err := store.GetSearchConsoleOverview(ctx, combined); err != nil || result.Clicks != 2 || result.Impressions != 40 {
		t.Fatalf("expected page and country filters to use query rows, got result=%+v err=%v", result, err)
	}
}

func TestSearchConsoleCompletedEmptyDaysFollowReplacementAndReset(t *testing.T) {
	ctx := t.Context()
	store := newSearchConsoleTenantTestStore(t)
	siteID := uuid.New()
	date := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	scope := SearchConsoleFactScope{
		SiteID: siteID, PropertyURI: "sc-domain:empty.example", StartDate: date, EndDate: date.AddDate(0, 0, 1),
		DataState: "final", TotalsComplete: true,
	}
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, nil); err != nil {
		t.Fatalf("import empty totals: %v", err)
	}
	days, err := store.ListSearchConsoleAudienceTotalsDays(ctx, siteID, scope.PropertyURI, scope.StartDate, scope.EndDate)
	if err != nil || len(days) != 2 {
		t.Fatalf("expected two completed empty days, got %v, err=%v", days, err)
	}
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, nil); err != nil {
		t.Fatalf("repeat empty import: %v", err)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, `SELECT count(*) FROM search_console_totals_days WHERE site_id = ?`, siteID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("repeated import duplicated checkpoints: count=%d, err=%v", count, err)
	}
	otherState := scope
	otherState.DataState = "all"
	if err := store.ReplaceSearchConsoleFacts(ctx, otherState, nil); err != nil {
		t.Fatalf("import non-final totals: %v", err)
	}
	days, err = store.ListSearchConsoleAudienceTotalsDays(ctx, siteID, scope.PropertyURI, date, date.AddDate(0, 0, 1))
	if err != nil || len(days) != 2 {
		t.Fatalf("non-final import changed final checkpoints: %v, err=%v", days, err)
	}
	for _, other := range []SearchConsoleFactScope{
		{SiteID: uuid.New(), PropertyURI: scope.PropertyURI, StartDate: date, EndDate: date},
		{SiteID: siteID, PropertyURI: "sc-domain:other.example", StartDate: date, EndDate: date},
	} {
		days, err := store.ListSearchConsoleAudienceTotalsDays(ctx, other.SiteID, other.PropertyURI, other.StartDate, other.EndDate)
		if err != nil || len(days) != 0 {
			t.Fatalf("checkpoint leaked into another scope: %v, err=%v", days, err)
		}
	}

	// A legacy replacement invalidates only the replaced date's completion.
	scope.EndDate, scope.TotalsComplete = date, false
	if err := store.ReplaceSearchConsoleFacts(ctx, scope, nil); err != nil {
		t.Fatalf("replace without totals: %v", err)
	}
	days, err = store.ListSearchConsoleAudienceTotalsDays(ctx, siteID, scope.PropertyURI, date, date.AddDate(0, 0, 1))
	if err != nil || len(days) != 1 || !days[0].Equal(date.AddDate(0, 0, 1)) {
		t.Fatalf("expected only the untouched day's checkpoint, got %v, err=%v", days, err)
	}
	if _, err := store.resetSiteAnalyticsMeasurements(ctx, siteID); err != nil {
		t.Fatalf("reset empty imports: %v", err)
	}
	days, err = store.ListSearchConsoleAudienceTotalsDays(ctx, siteID, scope.PropertyURI, date, date.AddDate(0, 0, 1))
	if err != nil || len(days) != 0 {
		t.Fatalf("reset left completed days behind: %v, err=%v", days, err)
	}
}

func newSearchConsoleTenantTestStore(t *testing.T) *Store {
	t.Helper()
	return newTenantTestFixtureStore(t)
}
