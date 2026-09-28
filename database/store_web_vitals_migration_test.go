package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/database/migrations"
	tenant "hitkeep/database/migrations/tenant"
)

func TestWebVitalsMetricIDMigrationUpgradesLegacyTable(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	createLegacyWebVitalsTable(t, store)
	contents, err := migrations.Fs.ReadFile("2026_06_01_010000_add_web_vital_metric_id.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, string(contents)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}

	assertWebVitalsMetricIDColumn(t, store)
	if _, err := store.DB().ExecContext(ctx, `
		INSERT INTO web_vitals (id, site_id, session_id, page_id, metric, metric_id, value, rating, path, timestamp)
		VALUES (?, ?, ?, ?, 'LCP', ?, 1200, 'good', '/pricing', ?)
	`, uuid.New(), uuid.New(), uuid.New(), uuid.New(), "v5-upgraded", time.Now().UTC()); err != nil {
		t.Fatalf("insert upgraded web vital: %v", err)
	}
}

func TestTenantWebVitalsMetricIDMigrationUpgradesLegacyTable(t *testing.T) {
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	createLegacyWebVitalsTable(t, store)
	contents, err := tenant.Fs.ReadFile("0011_add_web_vital_metric_id.sql")
	if err != nil {
		t.Fatalf("read tenant migration: %v", err)
	}
	if _, err := store.DB().ExecContext(ctx, string(contents)); err != nil {
		t.Fatalf("apply tenant migration: %v", err)
	}

	assertWebVitalsMetricIDColumn(t, store)
}

func createLegacyWebVitalsTable(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.DB().ExecContext(context.Background(), `
		CREATE TABLE web_vitals (
			id              UUID        PRIMARY KEY,
			site_id         UUID        NOT NULL,
			session_id      UUID        NOT NULL,
			page_id         UUID        NOT NULL,
			metric          VARCHAR     NOT NULL,
			value           DOUBLE      NOT NULL,
			rating          VARCHAR     NOT NULL,
			path            VARCHAR     NOT NULL,
			navigation_type VARCHAR,
			timestamp       TIMESTAMPTZ NOT NULL,
			tracker_source  VARCHAR,
			tracker_version VARCHAR
		)
	`); err != nil {
		t.Fatalf("create legacy web_vitals table: %v", err)
	}
}

func assertWebVitalsMetricIDColumn(t *testing.T, store *Store) {
	t.Helper()
	rows, err := store.DB().QueryContext(context.Background(), `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_name = 'web_vitals' AND column_name = 'metric_id'
	`)
	if err != nil {
		t.Fatalf("list web_vitals columns: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("expected metric_id column on web_vitals")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read web_vitals columns: %v", err)
	}
}
