package database

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
)

func TestIdempotentIngestBulkWritesReturnOnlyNewRows(t *testing.T) {
	store, _, site := setupAppenderStore(t)
	defer store.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	hit := &api.Hit{ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), PageID: uuid.New(), Timestamp: now, Path: "/signup"}
	event := &api.Event{ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), Timestamp: now, Name: "signup"}

	for name, run := range map[string]func() (int, error){
		"hit": func() (int, error) {
			created, err := store.CreateHitsBulkIdempotent(ctx, []*api.Hit{hit, hit})
			return len(created), err
		},
		"event": func() (int, error) {
			created, err := store.CreateEventsBulkIdempotent(ctx, []*api.Event{event, event})
			return len(created), err
		},
	} {
		t.Run(name, func(t *testing.T) {
			first, err := run()
			if err != nil || first != 1 {
				t.Fatalf("first write: created=%d err=%v", first, err)
			}
			second, err := run()
			if err != nil || second != 0 {
				t.Fatalf("retry write: created=%d err=%v", second, err)
			}
		})
	}
}
