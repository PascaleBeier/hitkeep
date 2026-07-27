package migrationacceptance_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

func TestLegacyTenantAnalyticsDrainMovesMissingRowsAndIsIdempotent(t *testing.T) {
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()
	root := t.TempDir()
	sharedPath := filepath.Join(root, "hitkeep.db")
	dataPath := filepath.Join(root, "data")
	tenantID := uuid.New()
	userID := uuid.New()
	siteID := uuid.New()
	bucketOne := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	bucketTwo := bucketOne.Add(time.Hour)
	extraBucket := bucketTwo.Add(time.Hour)

	shared, err := database.OpenMigratedStore(ctx, sharedPath)
	if err != nil {
		t.Fatal(err)
	}
	seedStatements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO users (id, email, password, created_at)
			VALUES (?, 'drain-test@example.invalid', 'disabled', ?)`, []any{userID, bucketOne}},
		{`INSERT INTO tenants (id, name, is_default) VALUES (?, 'Drain test', false)`, []any{tenantID}},
		{`INSERT INTO sites (id, user_id, domain, created_at, data_retention_days)
			VALUES (?, ?, 'drain-test.example.invalid', ?, 90)`, []any{siteID, userID, bucketOne}},
		{`INSERT INTO site_tenants (site_id, tenant_id) VALUES (?, ?)`, []any{siteID, tenantID}},
		{`INSERT INTO site_activity_hourly_counts
			(site_id, tenant_id, bucket, hits, events, updated_at)
			VALUES (?, ?, ?, 11, 4, ?), (?, ?, ?, 13, 6, ?)`,
			[]any{siteID, tenantID, bucketOne, bucketOne, siteID, tenantID, bucketTwo, bucketTwo}},
	}
	for _, statement := range seedStatements {
		if _, err := shared.DB().ExecContext(ctx, statement.query, statement.args...); err != nil {
			_ = shared.Close()
			t.Fatal(err)
		}
	}
	if err := shared.Close(); err != nil {
		t.Fatal(err)
	}

	tenantPath := filepath.Join(dataPath, "tenants", tenantID.String(), "hitkeep.db")
	if err := os.MkdirAll(filepath.Dir(tenantPath), 0o755); err != nil {
		t.Fatal(err)
	}
	tenant, err := database.OpenMigratedTenantStore(ctx, tenantPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tenant.DB().ExecContext(ctx,
		`INSERT INTO sites (id, domain, data_retention_days) VALUES (?, 'old.example.invalid', 30)`, siteID); err != nil {
		_ = tenant.Close()
		t.Fatal(err)
	}
	_, err = tenant.DB().ExecContext(ctx, `
		INSERT INTO site_activity_hourly_counts
			(site_id, tenant_id, bucket, hits, events, updated_at)
		VALUES (?, ?, ?, 11, 4, ?), (?, ?, ?, 17, 8, ?)`,
		siteID, tenantID, bucketOne, bucketOne, siteID, tenantID, extraBucket, extraBucket)
	if err != nil {
		_ = tenant.Close()
		t.Fatal(err)
	}
	if err := tenant.Close(); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if err := database.DrainLegacyTenantAnalytics(ctx, sharedPath, dataPath); err != nil {
			t.Fatal(err)
		}
	}

	shared, err = database.OpenMigratedStore(ctx, sharedPath)
	if err != nil {
		t.Fatal(err)
	}
	defer shared.Close()
	var sharedRows int
	if err := shared.DB().QueryRowContext(ctx, "SELECT count(*) FROM site_activity_hourly_counts").Scan(&sharedRows); err != nil {
		t.Fatal(err)
	}
	if sharedRows != 0 {
		t.Fatalf("legacy control retained %d activity rows", sharedRows)
	}

	tenant, err = database.OpenMigratedTenantStore(ctx, tenantPath)
	if err != nil {
		t.Fatal(err)
	}
	defer tenant.Close()
	var rows, hits, events int
	if err := tenant.DB().QueryRowContext(ctx, `
		SELECT count(*), sum(hits), sum(events)
		FROM site_activity_hourly_counts WHERE site_id = ?`, siteID).Scan(&rows, &hits, &events); err != nil {
		t.Fatal(err)
	}
	if rows != 3 || hits != 41 || events != 18 {
		t.Fatalf("unexpected tenant activity after drain: rows=%d hits=%d events=%d", rows, hits, events)
	}
	var domain string
	var retention int
	if err := tenant.DB().QueryRowContext(ctx, "SELECT domain, data_retention_days FROM sites WHERE id = ?", siteID).Scan(&domain, &retention); err != nil {
		t.Fatal(err)
	}
	if domain != "drain-test.example.invalid" || retention != 90 {
		t.Fatalf("site mirror was not synchronized: domain=%q retention=%d", domain, retention)
	}
}
