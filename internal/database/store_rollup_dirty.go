package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

type dirtyRollupType string

const (
	dirtyRollupHit     dirtyRollupType = "hit"
	dirtyRollupSession dirtyRollupType = "session"
)

type dirtyRollupBucket struct {
	SiteID     uuid.UUID
	RollupType dirtyRollupType
	BucketUnit string
	Bucket     time.Time
}

type bucketRange struct {
	Start time.Time
	End   time.Time
}

func dirtyBucketsForHit(hit *api.Hit) []dirtyRollupBucket {
	if hit == nil || hit.SiteID == uuid.Nil || hit.Timestamp.IsZero() {
		return nil
	}

	var buckets []dirtyRollupBucket
	for _, rollupType := range []dirtyRollupType{dirtyRollupHit, dirtyRollupSession} {
		for _, unit := range []string{"hour", "day", "month"} {
			buckets = append(buckets, dirtyRollupBucket{
				SiteID:     hit.SiteID,
				RollupType: rollupType,
				BucketUnit: unit,
				Bucket:     truncToUnit(hit.Timestamp.UTC(), unit),
			})
		}
	}
	return buckets
}

type dirtyRollupBucketKey struct {
	siteID     uuid.UUID
	rollupType dirtyRollupType
	bucketUnit string
	bucketUnix int64
}

func (s *Store) markDirtyRollupBuckets(ctx context.Context, buckets []dirtyRollupBucket) error {
	if len(buckets) == 0 {
		return nil
	}

	// Dedup, then upsert every distinct bucket in one multi-row statement —
	// a batch of hits marks the same handful of buckets over and over.
	seen := make(map[dirtyRollupBucketKey]struct{}, len(buckets))
	now := time.Now().UTC()
	args := make([]any, 0, len(buckets)*5)
	for _, bucket := range buckets {
		if bucket.SiteID == uuid.Nil || bucket.BucketUnit == "" || bucket.RollupType == "" || bucket.Bucket.IsZero() {
			continue
		}

		key := dirtyRollupBucketKey{
			siteID:     bucket.SiteID,
			rollupType: bucket.RollupType,
			bucketUnit: bucket.BucketUnit,
			bucketUnix: bucket.Bucket.UTC().UnixNano(),
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		args = append(args, bucket.SiteID, string(bucket.RollupType), bucket.BucketUnit, bucket.Bucket.UTC(), now)
	}
	if len(args) == 0 {
		return nil
	}

	placeholders := strings.Repeat(", (?, ?, ?, ?, ?)", len(args)/5)[2:]
	// #nosec G202 -- placeholders is only repeated "(?, ?, ?, ?, ?)" groups; every value is a bound arg.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO rollup_dirty_buckets (site_id, rollup_type, bucket_unit, bucket, updated_at)
		VALUES `+placeholders+`
		ON CONFLICT (site_id, rollup_type, bucket_unit, bucket) DO UPDATE SET
			updated_at = EXCLUDED.updated_at
	`, args...); err != nil {
		return fmt.Errorf("mark dirty rollup buckets: %w", err)
	}

	return nil
}

func (s *Store) listDirtyRollupBuckets(ctx context.Context, siteID uuid.UUID, rollupType dirtyRollupType, bucketUnit string, start time.Time, end time.Time) ([]time.Time, error) {
	if siteID == uuid.Nil || bucketUnit == "" || rollupType == "" {
		return nil, nil
	}

	query := `
		SELECT bucket
		FROM rollup_dirty_buckets
		WHERE site_id = ? AND rollup_type = ? AND bucket_unit = ?
	`
	args := []any{siteID, string(rollupType), bucketUnit}
	if !start.IsZero() && !end.IsZero() {
		query += " AND bucket >= ? AND bucket <= ?"
		args = append(args, truncToUnit(start.UTC(), bucketUnit), truncToUnit(end.UTC(), bucketUnit))
	}
	query += " ORDER BY bucket ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list dirty rollup buckets: %w", err)
	}
	defer rows.Close()

	var buckets []time.Time
	for rows.Next() {
		var bucket time.Time
		if err := rows.Scan(&bucket); err != nil {
			return nil, fmt.Errorf("scan dirty rollup bucket: %w", err)
		}
		buckets = append(buckets, truncToUnit(bucket.UTC(), bucketUnit))
	}
	return buckets, rows.Err()
}

func buildBucketRanges(buckets []time.Time, unit string) []bucketRange {
	if len(buckets) == 0 {
		return nil
	}

	ranges := make([]bucketRange, 0, len(buckets))
	current := bucketRange{Start: buckets[0], End: buckets[0]}
	for _, bucket := range buckets[1:] {
		expected := addUnit(current.End, unit, 1)
		if bucket.Equal(expected) {
			current.End = bucket
			continue
		}
		ranges = append(ranges, current)
		current = bucketRange{Start: bucket, End: bucket}
	}
	ranges = append(ranges, current)
	return ranges
}

func (s *Store) clearDirtyRollupBucketRange(
	ctx context.Context,
	siteID uuid.UUID,
	rollupType dirtyRollupType,
	bucketUnit string,
	start time.Time,
	end time.Time,
	refreshedBeforeOrAt time.Time,
) error {
	if siteID == uuid.Nil || rollupType == "" || bucketUnit == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM rollup_dirty_buckets
		WHERE site_id = ? AND rollup_type = ? AND bucket_unit = ? AND bucket >= ? AND bucket <= ? AND updated_at <= ?
	`, siteID, string(rollupType), bucketUnit, start.UTC(), end.UTC(), refreshedBeforeOrAt.UTC())
	if err != nil {
		return fmt.Errorf("clear dirty rollup buckets: %w", err)
	}
	return nil
}

func (s *Store) refreshDirtyRollupsInRange(ctx context.Context, siteID uuid.UUID, rollupType dirtyRollupType, kind rollupKind, start time.Time, end time.Time) error {
	unit := kindToTruncUnit(kind)
	refreshStartedAt := time.Now().UTC()
	buckets, err := s.listDirtyRollupBuckets(ctx, siteID, rollupType, unit, start, end)
	if err != nil || len(buckets) == 0 {
		return err
	}

	insertFn, err := rollupRefreshInsertFn(s, rollupType, kind)
	if err != nil {
		return err
	}

	for _, bucketRange := range buildBucketRanges(buckets, unit) {
		if err := insertFn(ctx, siteID, bucketRange.Start, bucketRange.End); err != nil {
			return err
		}
		if err := s.clearDirtyRollupBucketRange(ctx, siteID, rollupType, unit, bucketRange.Start, bucketRange.End, refreshStartedAt); err != nil {
			return err
		}
	}

	return nil
}

func (s *Store) ProcessDirtyRollups(ctx context.Context, siteID uuid.UUID) error {
	for _, rollupType := range []dirtyRollupType{dirtyRollupHit, dirtyRollupSession} {
		for _, kind := range []rollupKind{rollupHourly, rollupDaily, rollupMonthly} {
			if err := s.refreshDirtyRollupsInRange(ctx, siteID, rollupType, kind, time.Time{}, time.Time{}); err != nil {
				return err
			}
		}
	}
	return nil
}

func rollupRefreshInsertFn(s *Store, rollupType dirtyRollupType, kind rollupKind) (func(context.Context, uuid.UUID, time.Time, time.Time) error, error) {
	switch rollupType {
	case dirtyRollupHit:
		switch kind {
		case rollupHourly:
			return s.insertHourlyRollups, nil
		case rollupDaily:
			return s.insertDailyRollups, nil
		case rollupMonthly:
			return s.insertMonthlyRollups, nil
		}
	case dirtyRollupSession:
		switch kind {
		case rollupHourly:
			return s.insertHourlySessionRollups, nil
		case rollupDaily:
			return s.insertDailySessionRollups, nil
		case rollupMonthly:
			return s.insertMonthlySessionRollups, nil
		}
	}

	return nil, fmt.Errorf("unsupported rollup refresh kind %q/%q", rollupType, kind)
}
