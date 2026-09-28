package database

import (
	"context"
	"fmt"
	"time"

	duckdb "github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"

	"hitkeep/api"
)

func (s *Store) CreateEvent(ctx context.Context, event *api.Event) error {
	if event == nil {
		return fmt.Errorf("event is required")
	}
	return s.CreateEventsBulk(ctx, []*api.Event{event})
}

func (s *Store) CreateEventsBulk(ctx context.Context, events []*api.Event) error {
	if len(events) == 0 {
		return nil
	}

	return s.withAppender(ctx, "events", func(appender rowAppender) error {
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

			if err := appender.AppendRow(
				duckdb.UUID(event.ID),
				duckdb.UUID(event.SiteID),
				duckdb.UUID(event.SessionID),
				event.Name,
				event.Properties,
				event.Timestamp,
			); err != nil {
				return fmt.Errorf("append event row: %w", err)
			}
		}

		return nil
	})
}
