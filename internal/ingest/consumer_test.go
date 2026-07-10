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
	"hitkeep/internal/realtime"
	"hitkeep/internal/webhooks"
)

func testBatchLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestGoalConversionEventsAreStableAndPrivacySafe(t *testing.T) {
	t.Parallel()
	siteID := uuid.New()
	goalID := uuid.New()
	sourceID := uuid.New()
	occurredAt := time.Now().UTC()
	goals := []api.Goal{
		{ID: goalID, SiteID: siteID, Name: "Signup", Type: "event", Value: "signup"},
		{ID: uuid.New(), SiteID: siteID, Name: "Other", Type: "event", Value: "other"},
	}

	first := goalConversionEvents(goals, sourceID, siteID, "event", "signup", occurredAt)
	second := goalConversionEvents(goals, sourceID, siteID, "event", "signup", occurredAt)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one goal conversion, got first=%+v second=%+v", first, second)
	}
	if first[0].Type != webhooks.EventGoalConverted || first[0].ID == uuid.Nil || first[0].ID != second[0].ID {
		t.Fatalf("expected stable goal.converted event ID, got first=%+v second=%+v", first[0], second[0])
	}
	if first[0].Data["goal_id"] != goalID.String() || first[0].Data["goal_name"] != "Signup" {
		t.Fatalf("unexpected conversion summary: %+v", first[0].Data)
	}
	if _, ok := first[0].Data["properties"]; ok {
		t.Fatalf("conversion payload must not contain raw event properties: %+v", first[0].Data)
	}
}

type conversionBatchEmitter struct {
	hasSubscribers bool
	checks         int
	emitCalls      int
	batchCalls     int
	events         []webhooks.Event
	err            error
}

func (e *conversionBatchEmitter) HasSubscribers(context.Context, *uuid.UUID, string) (bool, error) {
	e.checks++
	return e.hasSubscribers, nil
}

func (e *conversionBatchEmitter) Emit(_ context.Context, event webhooks.Event) (webhooks.Emission, error) {
	e.emitCalls++
	e.events = append(e.events, event)
	return webhooks.Emission{EventID: event.ID}, nil
}

func (e *conversionBatchEmitter) EmitBatch(_ context.Context, events []webhooks.Event) ([]webhooks.Emission, error) {
	e.batchCalls++
	e.events = append(e.events, events...)
	return make([]webhooks.Emission, len(events)), e.err
}

func TestConsumerPropagatesConversionOutboxFailureForSafeRetry(t *testing.T) {
	expected := errors.New("outbox unavailable")
	emitter := &conversionBatchEmitter{hasSubscribers: true, err: expected}
	consumer := &Consumer{logger: testBatchLogger(), webhookEmitter: emitter}
	err := consumer.emitGoalConversions(context.Background(), []webhooks.Event{{ID: uuid.New(), Type: webhooks.EventGoalConverted}})
	if !errors.Is(err, expected) {
		t.Fatalf("expected outbox error to requeue ingest, got %v", err)
	}
}

func TestConsumerRetryAfterConversionOutboxFailureDoesNotDuplicateSourceEvent(t *testing.T) {
	ctx := context.Background()
	store := setupConsumerStore(t)
	mgr := database.NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })
	userID, err := store.CreateUser(ctx, "conversion-retry@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "conversion-retry.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	if err := store.CreateGoal(ctx, &api.Goal{SiteID: site.ID, Name: "Signup", Type: "event", Value: "signup"}); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	source := api.Event{ID: uuid.New(), SiteID: site.ID, SessionID: uuid.New(), Name: "signup", Timestamp: time.Now().UTC()}
	expected := errors.New("outbox unavailable")
	emitter := &conversionBatchEmitter{hasSubscribers: true, err: expected}
	consumer := NewConsumer(mgr, testBatchLogger(), slog.LevelWarn, nil)
	consumer.SetWebhookEmitter(emitter)
	t.Cleanup(consumer.Stop)

	if err := consumer.eventBatcher.persist(store, ctx, []*api.Event{&source}); !errors.Is(err, expected) {
		t.Fatalf("expected first attempt to requeue: %v", err)
	}
	emitter.err = nil
	if err := consumer.eventBatcher.persist(store, ctx, []*api.Event{&source}); err != nil {
		t.Fatalf("retry event: %v", err)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM events WHERE id = ?", source.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("source retry was not idempotent: count=%d err=%v", count, err)
	}
	if emitter.batchCalls != 2 || len(emitter.events) != 2 || emitter.events[0].ID != emitter.events[1].ID {
		t.Fatalf("conversion retry did not preserve stable event identity: %+v", emitter)
	}
}

func TestConsumerSkipsGoalLookupWithoutConversionSubscribers(t *testing.T) {
	emitter := &conversionBatchEmitter{hasSubscribers: false}
	consumer := &Consumer{logger: testBatchLogger(), webhookEmitter: emitter}
	consumer.emitHitGoalConversions(context.Background(), &database.Store{}, []*api.Hit{{SiteID: uuid.New(), Path: "/signup"}})

	if emitter.checks != 1 || emitter.emitCalls != 0 || emitter.batchCalls != 0 {
		t.Fatalf("unexpected emission work: %+v", emitter)
	}
}

func TestConsumerBulkEmitsGoalConversions(t *testing.T) {
	ctx := context.Background()
	store := setupConsumerStore(t)
	userID, err := store.CreateUser(ctx, "conversion-batch@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "conversion-batch.example.com")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	siteID := site.ID
	if err := store.CreateGoal(ctx, &api.Goal{SiteID: siteID, Name: "Signup", Type: "event", Value: "signup"}); err != nil {
		t.Fatalf("create goal: %v", err)
	}
	emitter := &conversionBatchEmitter{hasSubscribers: true}
	consumer := &Consumer{logger: testBatchLogger(), webhookEmitter: emitter}
	consumer.emitEventGoalConversions(ctx, store, []*api.Event{
		{ID: uuid.New(), SiteID: siteID, Name: "signup", Timestamp: time.Now()},
		{ID: uuid.New(), SiteID: siteID, Name: "signup", Timestamp: time.Now()},
	})

	if emitter.checks != 1 || emitter.batchCalls != 1 || emitter.emitCalls != 0 || len(emitter.events) != 2 {
		t.Fatalf("expected one batch emission for two conversions, got %+v", emitter)
	}
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

	consumer := NewConsumer(mgr, testBatchLogger(), slog.LevelWarn, nil)
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

	consumer := NewConsumer(mgr, testBatchLogger(), slog.LevelWarn, nil)
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

func TestConsumerPublishesRealtimeAfterPersistingEvents(t *testing.T) {
	ctx := context.Background()
	store := setupConsumerStore(t)
	mgr := database.NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "consumer-realtime@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "consumer-realtime.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	broker := realtime.NewBroker()
	sub, _, _ := broker.Subscribe(site.ID, "")
	defer sub.Close()

	canonical := time.Date(2026, 4, 4, 8, 15, 0, 0, time.UTC)
	event := api.Event{
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "purchase",
		Properties: map[string]any{"value": 42},
		Timestamp:  canonical,
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}

	consumer := NewConsumer(mgr, testBatchLogger(), slog.LevelWarn, broker)
	t.Cleanup(consumer.Stop)
	if err := consumer.handleEvent(newConsumerTestMessage(body)); err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	select {
	case changed := <-sub.Events():
		if changed.Name != realtime.EventAnalyticsChanged {
			t.Fatalf("expected changed event, got %q", changed.Name)
		}
		if changed.Counts[realtime.KindEvents] != 1 || changed.Counts[realtime.KindEcommerce] != 1 {
			t.Fatalf("expected event and ecommerce counts, got %+v", changed.Counts)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for realtime event")
	}
}

func TestConsumerPersistsWebVitalCanonicalTimestampFromMessage(t *testing.T) {
	ctx := context.Background()
	store := setupConsumerStore(t)
	mgr := database.NewTenantStoreManager(store, t.TempDir())
	t.Cleanup(func() { _ = mgr.Close() })

	userID, err := store.CreateUser(ctx, "consumer-vital@example.com", "hash")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "consumer-vital.example.com")
	if err != nil {
		t.Fatalf("CreateSite: %v", err)
	}

	canonical := time.Date(2026, 4, 5, 9, 45, 0, 0, time.UTC)
	navType := "navigate"
	vital := api.WebVital{
		SiteID:         site.ID,
		SessionID:      uuid.New(),
		PageID:         uuid.New(),
		Metric:         api.WebVitalLCP,
		Value:          4100,
		Path:           "/pricing",
		NavigationType: &navType,
		Timestamp:      canonical,
		TrackerSource:  "browser",
		TrackerVersion: "dev",
	}
	body, err := json.Marshal(vital)
	if err != nil {
		t.Fatalf("marshal web vital: %v", err)
	}

	consumer := NewConsumer(mgr, testBatchLogger(), slog.LevelWarn, nil)
	t.Cleanup(consumer.Stop)
	if err := consumer.handleWebVital(newConsumerTestMessage(body)); err != nil {
		t.Fatalf("handleWebVital: %v", err)
	}

	summary, err := store.GetWebVitalsSummary(ctx, api.WebVitalsParams{
		SiteID: site.ID,
		Start:  canonical.Add(-time.Minute),
		End:    canonical.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("GetWebVitalsSummary: %v", err)
	}
	if len(summary) != 1 {
		t.Fatalf("expected 1 summary metric, got %d: %+v", len(summary), summary)
	}
	got := summary[0]
	if got.Metric != api.WebVitalLCP {
		t.Fatalf("expected LCP metric, got %q", got.Metric)
	}
	if got.Rating != api.WebVitalRatingPoor {
		t.Fatalf("expected poor rating, got %q", got.Rating)
	}
	if got.P75 != 4100 {
		t.Fatalf("expected p75 4100, got %f", got.P75)
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
