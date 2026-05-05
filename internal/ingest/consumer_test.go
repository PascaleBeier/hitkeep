package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

func testBatchLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestStoreBatcherFlushesByStore(t *testing.T) {
	storeA := &database.Store{}
	storeB := &database.Store{}

	var mu sync.Mutex
	flushed := make(map[*database.Store]int)

	batcher := newStoreBatcher("hit", testBatchLogger(), 3, time.Hour, time.Second, func(store *database.Store, _ context.Context, hits []*api.Hit) error {
		mu.Lock()
		defer mu.Unlock()
		flushed[store] += len(hits)
		return nil
	})
	defer batcher.Stop()

	results := make([]<-chan error, 0, 3)
	for _, item := range []batchItem[*api.Hit]{
		{store: storeA, siteID: uuid.New(), value: &api.Hit{Path: "/pricing"}},
		{store: storeA, siteID: uuid.New(), value: &api.Hit{Path: "/signup"}},
		{store: storeB, siteID: uuid.New(), value: &api.Hit{Path: "/docs"}},
	} {
		result, err := batcher.Enqueue(item)
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		results = append(results, result)
	}

	for _, result := range results {
		select {
		case err := <-result:
			if err != nil {
				t.Fatalf("unexpected batch error: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for batched flush")
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if flushed[storeA] != 2 {
		t.Fatalf("expected storeA to flush 2 hits, got %d", flushed[storeA])
	}
	if flushed[storeB] != 1 {
		t.Fatalf("expected storeB to flush 1 hit, got %d", flushed[storeB])
	}
}

func TestStoreBatcherFlushesOnIntervalAndPropagatesError(t *testing.T) {
	expectedErr := errors.New("boom")
	store := &database.Store{}

	batcher := newStoreBatcher("event", testBatchLogger(), 10, 10*time.Millisecond, time.Second, func(_ *database.Store, _ context.Context, _ []*api.Event) error {
		return expectedErr
	})
	defer batcher.Stop()

	result, err := batcher.Enqueue(batchItem[*api.Event]{
		store:  store,
		siteID: uuid.New(),
		value:  &api.Event{Name: "signup"},
	})
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	select {
	case gotErr := <-result:
		if !errors.Is(gotErr, expectedErr) {
			t.Fatalf("expected %v, got %v", expectedErr, gotErr)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for interval flush")
	}
}

func TestConsumerPersistsHitCanonicalTimestampFromMessage(t *testing.T) {
	ctx := context.Background()
	store := setupConsumerStore(t)
	mgr := database.NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "consumer-hit@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "consumer-hit.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	canonical := time.Date(2026, 4, 3, 12, 30, 45, 0, time.UTC)
	hit := api.Hit{
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: canonical,
		Path:      "/docs",
	}
	body, err := json.Marshal(hit)
	if err != nil {
		t.Fatalf("marshal hit: %v", err)
	}

	consumer := NewConsumer(mgr, testBatchLogger(), slog.LevelWarn)
	t.Cleanup(consumer.Stop)
	if err := consumer.handleHit(newConsumerTestMessage(body)); err != nil {
		t.Fatalf("handleHit: %v", err)
	}

	hits, err := store.GetHits(ctx, api.HitQueryParams{
		SiteID: site.ID,
		Start:  canonical.Add(-time.Minute),
		End:    canonical.Add(time.Minute),
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("GetHits: %v", err)
	}
	if hits.Total != 1 {
		t.Fatalf("expected 1 persisted hit, got %d", hits.Total)
	}
	if !hits.Data[0].Timestamp.Equal(canonical) {
		t.Fatalf("expected timestamp %s, got %s", canonical, hits.Data[0].Timestamp)
	}
}

func TestConsumerPersistsEventCanonicalTimestampFromMessage(t *testing.T) {
	ctx := context.Background()
	store := setupConsumerStore(t)
	mgr := database.NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "consumer-event@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "consumer-event.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	canonical := time.Date(2026, 4, 4, 8, 15, 0, 0, time.UTC)
	event := api.Event{
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "signup_started",
		Properties: map[string]any{"plan": "pro"},
		Timestamp:  canonical,
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	consumer := NewConsumer(mgr, testBatchLogger(), slog.LevelWarn)
	t.Cleanup(consumer.Stop)
	if err := consumer.handleEvent(newConsumerTestMessage(body)); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	series, err := store.GetEventTimeseries(ctx, api.EventTimeseriesParams{
		SiteID:    site.ID,
		EventName: "signup_started",
		Start:     canonical.Add(-time.Minute),
		End:       canonical.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("GetEventTimeseries: %v", err)
	}
	var total int
	for _, point := range series {
		total += point.Count
	}
	if total != 1 {
		t.Fatalf("expected 1 persisted event in canonical range, got %d points=%+v", total, series)
	}
}

type noopMessageDelegate struct{}

func (noopMessageDelegate) OnFinish(*nsq.Message) {}

func (noopMessageDelegate) OnRequeue(*nsq.Message, time.Duration, bool) {}

func (noopMessageDelegate) OnTouch(*nsq.Message) {}

func newConsumerTestMessage(body []byte) *nsq.Message {
	msg := nsq.NewMessage(nsq.MessageID{}, body)
	msg.Delegate = noopMessageDelegate{}
	return msg
}

func setupConsumerStore(t *testing.T) *database.Store {
	t.Helper()
	store := database.NewStore(filepath.Join(t.TempDir(), "consumer.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return store
}
