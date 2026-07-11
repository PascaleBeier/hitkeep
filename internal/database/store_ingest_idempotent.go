package database

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

// CreateHitsBulkIdempotent inserts only source IDs that are not already
// durable. It is used by the NSQ ingest path so a post-persist outbox failure
// can safely requeue the original message batch.
func (s *Store) CreateHitsBulkIdempotent(ctx context.Context, hits []*api.Hit) ([]*api.Hit, error) {
	for _, hit := range hits {
		if hit == nil {
			continue
		}
		if hit.ID == uuid.Nil {
			hit.ID = uuid.New()
		}
		if hit.Timestamp.IsZero() {
			hit.Timestamp = time.Now()
		}
	}
	missing, err := filterMissingIngestRows(ctx, s, "hits", hits, func(hit *api.Hit) uuid.UUID {
		if hit == nil {
			return uuid.Nil
		}
		return hit.ID
	})
	if err != nil {
		return nil, err
	}
	if err := s.CreateHitsBulk(ctx, missing); err != nil {
		return nil, err
	}
	return missing, nil
}

func (s *Store) CreateEventsBulkIdempotent(ctx context.Context, events []*api.Event) ([]*api.Event, error) {
	for _, event := range events {
		if event == nil {
			continue
		}
		if event.ID == uuid.Nil {
			event.ID = uuid.New()
		}
		if event.Timestamp.IsZero() {
			event.Timestamp = time.Now()
		}
	}
	missing, err := filterMissingIngestRows(ctx, s, "events", events, func(event *api.Event) uuid.UUID {
		if event == nil {
			return uuid.Nil
		}
		return event.ID
	})
	if err != nil {
		return nil, err
	}
	if err := s.CreateEventsBulk(ctx, missing); err != nil {
		return nil, err
	}
	return missing, nil
}

func filterMissingIngestRows[T any](ctx context.Context, store *Store, table string, rows []T, id func(T) uuid.UUID) ([]T, error) {
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if value := id(row); value != uuid.Nil {
			ids = append(ids, value)
		}
	}
	if len(ids) == 0 {
		return []T{}, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for index, value := range ids {
		args[index] = value
	}
	// table is selected only by CreateHitsBulkIdempotent and CreateEventsBulkIdempotent;
	// placeholders are generated locally and values remain bound parameters.
	existingRows, err := store.db.QueryContext(ctx, "SELECT id FROM "+table+" WHERE id IN ("+placeholders+")", args...) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("load existing %s ingest IDs: %w", table, err)
	}
	defer existingRows.Close()
	existing := make(map[uuid.UUID]struct{}, len(ids))
	for existingRows.Next() {
		var value uuid.UUID
		if err := existingRows.Scan(&value); err != nil {
			return nil, fmt.Errorf("scan existing %s ingest ID: %w", table, err)
		}
		existing[value] = struct{}{}
	}
	if err := existingRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing %s ingest IDs: %w", table, err)
	}
	missing := make([]T, 0, len(rows)-len(existing))
	for _, row := range rows {
		value := id(row)
		if value == uuid.Nil {
			continue
		}
		if _, ok := existing[value]; ok {
			continue
		}
		existing[value] = struct{}{}
		missing = append(missing, row)
	}
	return missing, nil
}
