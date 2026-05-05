package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
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
}

func TestUpsertSearchConsoleFactIsIdempotent(t *testing.T) {
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
	if err := store.UpsertSearchConsoleFact(ctx, input); err != nil {
		t.Fatalf("upsert first fact: %v", err)
	}
	input.Clicks = 5
	input.Impressions = 50
	input.CTR = 0.2
	input.Position = 3.8
	if err := store.UpsertSearchConsoleFact(ctx, input); err != nil {
		t.Fatalf("upsert second fact: %v", err)
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

func TestUpsertSearchConsoleFactKeepsDistinctAggregationTypes(t *testing.T) {
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
	if err := store.UpsertSearchConsoleFact(ctx, byProperty); err != nil {
		t.Fatalf("upsert byProperty fact: %v", err)
	}
	byPage := base
	byPage.Clicks = 7
	byPage.AggregationType = "byPage"
	if err := store.UpsertSearchConsoleFact(ctx, byPage); err != nil {
		t.Fatalf("upsert byPage fact: %v", err)
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

func newSearchConsoleTenantTestStore(t *testing.T) *Store {
	t.Helper()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.MigrateTenant(context.Background()); err != nil {
		t.Fatalf("migrate tenant store: %v", err)
	}
	return store
}
