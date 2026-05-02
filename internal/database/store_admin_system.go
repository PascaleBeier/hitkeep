package database

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// SystemCounter holds atomic counters updated during ingest.
type SystemCounter struct {
	Hits       atomic.Int64
	Events     atomic.Int64
	Rejections atomic.Int64
	Spam       atomic.Int64
}

type RecentIngestCounts struct {
	Hits   int
	Events int
}

// BackupStatusTracker holds the current state of the backup worker.
type BackupStatusTracker struct {
	mu             sync.Mutex
	LastBackup     *time.Time
	NextBackup     *time.Time
	LastFailedAt   *time.Time
	LastError      string
	RecentFailures int
	Enabled        bool
	ConfigPath     string
	IntervalMin    int
	Retention      int
}

// ImportStageCleanupStatusTracker holds the latest staged import cleanup run.
type ImportStageCleanupStatusTracker struct {
	mu                 sync.Mutex
	Enabled            bool
	RetentionDays      int
	LastRun            *time.Time
	LastFailedAt       *time.Time
	LastError          string
	RecentFailures     int
	LastCleanedImports int
	LastCleanedFiles   int
	LastCleanedBytes   int64
	LastMarkedFailed   int
}

func (b *BackupStatusTracker) SetLastBackup(t time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.LastBackup = &t
	b.LastError = ""
}

func (b *BackupStatusTracker) SetFailed(at time.Time, errStr string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.LastFailedAt = &at
	b.LastError = errStr
	b.RecentFailures++
}

func (b *BackupStatusTracker) SetNextBackup(t time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.NextBackup = &t
}

func (b *BackupStatusTracker) SetConfig(enabled bool, path string, interval, retention int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Enabled = enabled
	b.ConfigPath = path
	b.IntervalMin = interval
	b.Retention = retention
}

func (b *BackupStatusTracker) Status() api.SystemBackupStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	return api.SystemBackupStatus{
		Enabled:        b.Enabled,
		ConfigPath:     b.ConfigPath,
		IntervalMin:    b.IntervalMin,
		Retention:      b.Retention,
		LastBackup:     b.LastBackup,
		NextBackup:     b.NextBackup,
		LastFailedAt:   b.LastFailedAt,
		LastError:      b.LastError,
		RecentFailures: b.RecentFailures,
	}
}

func (t *ImportStageCleanupStatusTracker) SetConfig(enabled bool, retentionDays int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Enabled = enabled
	t.RetentionDays = retentionDays
}

func (t *ImportStageCleanupStatusTracker) SetLastRun(at time.Time, result api.ImportStageCleanupRunResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.LastRun = &at
	t.LastError = ""
	t.LastCleanedImports = result.ImportsCleaned
	t.LastCleanedFiles = result.FilesCleaned
	t.LastCleanedBytes = result.BytesCleaned
	t.LastMarkedFailed = result.ImportsMarkedFailed
}

func (t *ImportStageCleanupStatusTracker) SetFailed(at time.Time, errStr string, result api.ImportStageCleanupRunResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.LastFailedAt = &at
	t.LastError = errStr
	t.RecentFailures++
	t.LastCleanedImports = result.ImportsCleaned
	t.LastCleanedFiles = result.FilesCleaned
	t.LastCleanedBytes = result.BytesCleaned
	t.LastMarkedFailed = result.ImportsMarkedFailed
}

func (t *ImportStageCleanupStatusTracker) Status(estimate api.ImportStageCleanupEstimate) api.SystemImportStageCleanupStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return api.SystemImportStageCleanupStatus{
		Enabled:            t.Enabled,
		RetentionDays:      t.RetentionDays,
		StaleImports:       estimate.Imports,
		StaleFiles:         estimate.Files,
		StaleBytes:         estimate.Bytes,
		LastRun:            t.LastRun,
		LastFailedAt:       t.LastFailedAt,
		LastError:          t.LastError,
		RecentFailures:     t.RecentFailures,
		LastCleanedImports: t.LastCleanedImports,
		LastCleanedFiles:   t.LastCleanedFiles,
		LastCleanedBytes:   t.LastCleanedBytes,
		LastMarkedFailed:   t.LastMarkedFailed,
	}
}

// MailTestTracker holds the result of the last mail test.
type MailTestTracker struct {
	mu         sync.Mutex
	LastTestAt *time.Time
	LastTestOK *bool
}

func (m *MailTestTracker) SetResult(ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	m.LastTestAt = &now
	m.LastTestOK = &ok
}

func (m *MailTestTracker) Status() (lastTestAt *time.Time, lastTestOK *bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.LastTestAt, m.LastTestOK
}

func (s *Store) GetSiteCount(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sites").Scan(&count); err != nil {
		return 0, fmt.Errorf("could not count sites: %w", err)
	}
	return count, nil
}

func (s *Store) GetTenantList(ctx context.Context) ([]api.TenantDBInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT CAST(t.id AS VARCHAR), t.name
		FROM tenants t
		LEFT JOIN tenant_archives ta ON ta.tenant_id = t.id
		WHERE t.is_default = FALSE AND ta.tenant_id IS NULL
		ORDER BY t.name
	`)
	if err != nil {
		return nil, fmt.Errorf("could not list tenants: %w", err)
	}
	defer rows.Close()

	var tenants []api.TenantDBInfo
	for rows.Next() {
		var rawID string
		var name string
		if err := rows.Scan(&rawID, &name); err != nil {
			return nil, fmt.Errorf("could not scan tenant: %w", err)
		}
		id, err := uuid.Parse(rawID)
		if err != nil {
			return nil, fmt.Errorf("invalid tenant id: %w", err)
		}
		tenants = append(tenants, api.TenantDBInfo{
			TenantID: id,
			Name:     name,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("could not read tenant rows: %w", err)
	}
	return tenants, nil
}

func (s *Store) GetRecentHitsCount(ctx context.Context, since time.Time) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM hits WHERE timestamp >= ?", since,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("could not count recent hits: %w", err)
	}
	return count, nil
}

func (s *Store) GetRecentEventsCount(ctx context.Context, since time.Time) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM events WHERE timestamp >= ?", since,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("could not count recent events: %w", err)
	}
	return count, nil
}

func (s *Store) GetRecentIngestCounts(ctx context.Context, since time.Time) (RecentIngestCounts, error) {
	hits, err := s.GetRecentHitsCount(ctx, since)
	if err != nil {
		return RecentIngestCounts{}, err
	}
	events, err := s.GetRecentEventsCount(ctx, since)
	if err != nil {
		return RecentIngestCounts{}, err
	}
	return RecentIngestCounts{Hits: hits, Events: events}, nil
}

func (m *TenantStoreManager) GetRecentIngestCounts(ctx context.Context, since time.Time) (RecentIngestCounts, error) {
	if m == nil || m.shared == nil {
		return RecentIngestCounts{}, fmt.Errorf("tenant store manager is not configured")
	}

	total, err := m.shared.GetRecentIngestCounts(ctx, since)
	if err != nil {
		return RecentIngestCounts{}, fmt.Errorf("count shared ingest: %w", err)
	}

	tenants, err := m.shared.GetTenantList(ctx)
	if err != nil {
		return RecentIngestCounts{}, fmt.Errorf("list tenants for ingest counts: %w", err)
	}

	for _, tenant := range tenants {
		store, err := m.ForTenant(ctx, tenant.TenantID)
		if err != nil {
			return RecentIngestCounts{}, fmt.Errorf("open tenant store %s: %w", tenant.TenantID, err)
		}
		if store == m.shared {
			continue
		}

		counts, err := store.GetRecentIngestCounts(ctx, since)
		if err != nil {
			return RecentIngestCounts{}, fmt.Errorf("count tenant ingest %s: %w", tenant.TenantID, err)
		}
		total.Hits += counts.Hits
		total.Events += counts.Events
	}

	return total, nil
}
