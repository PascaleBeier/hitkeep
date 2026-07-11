package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/hklog"
	"hitkeep/internal/realtime"
	"hitkeep/internal/webhooks"
)

type Consumer struct {
	tenantMgr      *database.TenantStoreManager
	hitsConsumer   *nsq.Consumer
	eventConsumer  *nsq.Consumer
	vitalConsumer  *nsq.Consumer
	hitBatcher     *storeBatcher[*api.Hit]
	eventBatcher   *storeBatcher[*api.Event]
	vitalBatcher   *storeBatcher[*api.WebVital]
	realtime       *realtime.Broker
	logger         *slog.Logger
	logLevel       slog.Level
	webhookEmitter webhooks.EventEmitter
}

func NewConsumer(tenantMgr *database.TenantStoreManager, logger *slog.Logger, level slog.Level, realtimeBroker *realtime.Broker) *Consumer {
	consumer := &Consumer{
		tenantMgr: tenantMgr,
		logger:    logger,
		logLevel:  level,
		realtime:  realtimeBroker,
	}
	consumer.hitBatcher = newStoreBatcher("hit", logger, ingestBatchSize, ingestBatchFlushInterval, ingestPersistTimeout, func(store *database.Store, ctx context.Context, hits []*api.Hit) error {
		created, err := store.CreateHitsBulkIdempotent(ctx, hits)
		if err != nil {
			return err
		}
		if len(created) > 0 {
			if err := tenantMgr.Shared().RecordHitActivity(ctx, created); err != nil {
				logger.Warn("Failed to record hit activity summary after tenant persistence", "count", len(created), "error", err)
			}
			consumer.publishHitsChanged(created)
		}
		return consumer.emitHitGoalConversions(ctx, store, hits)
	})
	consumer.eventBatcher = newStoreBatcher("event", logger, ingestBatchSize, ingestBatchFlushInterval, ingestPersistTimeout, func(store *database.Store, ctx context.Context, events []*api.Event) error {
		created, err := store.CreateEventsBulkIdempotent(ctx, events)
		if err != nil {
			return err
		}
		if len(created) > 0 {
			if err := tenantMgr.Shared().RecordEventActivity(ctx, created); err != nil {
				logger.Warn("Failed to record event activity summary after tenant persistence", "count", len(created), "error", err)
			}
			consumer.publishEventsChanged(created)
		}
		return consumer.emitEventGoalConversions(ctx, store, events)
	})
	consumer.vitalBatcher = newStoreBatcher("web_vital", logger, ingestBatchSize, ingestBatchFlushInterval, ingestPersistTimeout, func(store *database.Store, ctx context.Context, vitals []*api.WebVital) error {
		if err := store.CreateWebVitalsBulk(ctx, vitals); err != nil {
			return err
		}
		consumer.publishWebVitalsChanged(vitals)
		return nil
	})
	return consumer
}

func (c *Consumer) SetWebhookEmitter(emitter webhooks.EventEmitter) {
	c.webhookEmitter = emitter
}

func (c *Consumer) emitHitGoalConversions(ctx context.Context, store *database.Store, hits []*api.Hit) error {
	if c.webhookEmitter == nil {
		return nil
	}
	sources := make([]goalConversionSource, 0, len(hits))
	for _, hit := range hits {
		if hit == nil {
			continue
		}
		sources = append(sources, goalConversionSource{id: hit.ID, siteID: hit.SiteID, sourceType: "path", value: hit.Path, occurredAt: hit.Timestamp})
	}
	return c.emitGoalConversionSources(ctx, store, "hit", sources)
}

func (c *Consumer) emitEventGoalConversions(ctx context.Context, store *database.Store, events []*api.Event) error {
	if c.webhookEmitter == nil {
		return nil
	}
	sources := make([]goalConversionSource, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		sources = append(sources, goalConversionSource{id: event.ID, siteID: event.SiteID, sourceType: "event", value: event.Name, occurredAt: event.Timestamp})
	}
	return c.emitGoalConversionSources(ctx, store, "event", sources)
}

type goalConversionSource struct {
	id         uuid.UUID
	siteID     uuid.UUID
	sourceType string
	value      string
	occurredAt time.Time
}

func (c *Consumer) emitGoalConversionSources(ctx context.Context, store *database.Store, sourceLabel string, sources []goalConversionSource) error {
	bySite := make(map[uuid.UUID][]api.Goal)
	subscriptions := make(map[uuid.UUID]bool)
	conversionEvents := make([]webhooks.Event, 0)
	for _, source := range sources {
		subscribed, checked := subscriptions[source.siteID]
		if !checked {
			subscribed = c.hasGoalConversionSubscribers(ctx, source.siteID)
			subscriptions[source.siteID] = subscribed
		}
		if !subscribed {
			continue
		}
		goals, ok := bySite[source.siteID]
		if !ok {
			var err error
			goals, err = store.GetGoals(ctx, source.siteID)
			if err != nil {
				return fmt.Errorf("load goals for %s webhook conversion: %w", sourceLabel, err)
			}
			bySite[source.siteID] = goals
		}
		conversionEvents = append(conversionEvents, goalConversionEvents(goals, source.id, source.siteID, source.sourceType, source.value, source.occurredAt)...)
	}
	return c.emitGoalConversions(ctx, conversionEvents)
}

func (c *Consumer) emitGoalConversions(ctx context.Context, events []webhooks.Event) error {
	if len(events) == 0 {
		return nil
	}
	if emitter, ok := c.webhookEmitter.(webhooks.BatchEventEmitter); ok {
		if _, err := emitter.EmitBatch(ctx, events); err != nil {
			return fmt.Errorf("persist goal conversion webhook batch: %w", err)
		}
		return nil
	}
	for _, event := range events {
		if _, err := c.webhookEmitter.Emit(ctx, event); err != nil {
			return fmt.Errorf("persist goal conversion webhook event %s: %w", event.ID, err)
		}
	}
	return nil
}

func (c *Consumer) hasGoalConversionSubscribers(ctx context.Context, siteID uuid.UUID) bool {
	checker, ok := c.webhookEmitter.(webhooks.SubscriptionChecker)
	if !ok {
		return true
	}
	hasSubscribers, err := checker.HasSubscribers(ctx, &siteID, webhooks.EventGoalConverted)
	if err != nil {
		c.logger.Warn("Failed to check goal conversion webhook subscriptions", "error", err, "site_id", siteID)
		return true
	}
	return hasSubscribers
}

func goalConversionEvents(goals []api.Goal, sourceID, siteID uuid.UUID, sourceType, value string, occurredAt time.Time) []webhooks.Event {
	result := make([]webhooks.Event, 0)
	for _, goal := range goals {
		if goal.SiteID != siteID || goal.Type != sourceType || goal.Value != value {
			continue
		}
		eventID := uuid.NewSHA1(uuid.NameSpaceOID, []byte(sourceID.String()+":"+goal.ID.String()+":"+webhooks.EventGoalConverted))
		result = append(result, webhooks.Event{
			ID:     eventID,
			Type:   webhooks.EventGoalConverted,
			SiteID: &siteID,
			Data: map[string]any{
				"site_id": siteID.String(), "goal_id": goal.ID.String(), "goal_name": goal.Name,
				"goal_type": goal.Type, "converted_at": occurredAt.UTC(),
			},
		})
	}
	return result
}

// newIngestConsumerConfig tunes delivery for the batching handlers: handlers
// hold each message only until its batch flush (≤200ms) plus the 10s persist
// timeout, so a 30s MsgTimeout redelivers wedged messages fast, and a 5s
// requeue delay retries transient persist failures promptly instead of after
// the 90s go-nsq default.
func newIngestConsumerConfig() *nsq.Config {
	config := nsq.NewConfig()
	config.MaxInFlight = ingestConsumerConcurrency
	config.MsgTimeout = 30 * time.Second
	config.MaxAttempts = 5
	config.DefaultRequeueDelay = 5 * time.Second
	return config
}

func (c *Consumer) Connect(addr string) error {
	// Hits Consumer
	hitsConsumer, err := nsq.NewConsumer("hits", "db-writer", newIngestConsumerConfig())
	if err != nil {
		return err
	}
	hitsConsumer.SetLogger(hklog.GoNSQLogger{Logger: c.logger}, hklog.NSQGoLevel(c.logLevel))
	hitsConsumer.AddConcurrentHandlers(nsq.HandlerFunc(c.handleHit), ingestConsumerConcurrency)
	if err := hitsConsumer.ConnectToNSQD(addr); err != nil {
		return err
	}
	c.hitsConsumer = hitsConsumer

	// Events Consumer
	eventConsumer, err := nsq.NewConsumer("events", "db-writer", newIngestConsumerConfig())
	if err != nil {
		return err
	}
	eventConsumer.SetLogger(hklog.GoNSQLogger{Logger: c.logger}, hklog.NSQGoLevel(c.logLevel))
	eventConsumer.AddConcurrentHandlers(nsq.HandlerFunc(c.handleEvent), ingestConsumerConcurrency)
	if err := eventConsumer.ConnectToNSQD(addr); err != nil {
		return err
	}
	c.eventConsumer = eventConsumer

	vitalConsumer, err := nsq.NewConsumer("web_vitals", "db-writer", newIngestConsumerConfig())
	if err != nil {
		return err
	}
	vitalConsumer.SetLogger(hklog.GoNSQLogger{Logger: c.logger}, hklog.NSQGoLevel(c.logLevel))
	vitalConsumer.AddConcurrentHandlers(nsq.HandlerFunc(c.handleWebVital), ingestConsumerConcurrency)
	if err := vitalConsumer.ConnectToNSQD(addr); err != nil {
		return err
	}
	c.vitalConsumer = vitalConsumer

	return nil
}

func (c *Consumer) Stop() {
	if c.hitsConsumer != nil {
		c.hitsConsumer.Stop()
		<-c.hitsConsumer.StopChan
	}
	if c.eventConsumer != nil {
		c.eventConsumer.Stop()
		<-c.eventConsumer.StopChan
	}
	if c.vitalConsumer != nil {
		c.vitalConsumer.Stop()
		<-c.vitalConsumer.StopChan
	}
	if c.hitBatcher != nil {
		c.hitBatcher.Stop()
	}
	if c.eventBatcher != nil {
		c.eventBatcher.Stop()
	}
	if c.vitalBatcher != nil {
		c.vitalBatcher.Stop()
	}
}

func (c *Consumer) handleHit(m *nsq.Message) error {
	return processMessage(m, c, c.hitBatcher, func(v *api.Hit) (uuid.UUID, []any) {
		return v.SiteID, []any{"path", v.Path}
	}, "hit")
}

func (c *Consumer) handleEvent(m *nsq.Message) error {
	return processMessage(m, c, c.eventBatcher, func(v *api.Event) (uuid.UUID, []any) {
		return v.SiteID, []any{"name", v.Name}
	}, "event")
}

func (c *Consumer) handleWebVital(m *nsq.Message) error {
	return processMessage(m, c, c.vitalBatcher, func(v *api.WebVital) (uuid.UUID, []any) {
		return v.SiteID, []any{"metric", v.Metric, "path", v.Path}
	}, "web vital")
}

type siteIdentifiable interface {
	api.Hit | api.Event | api.WebVital
}

func processMessage[T siteIdentifiable](
	m *nsq.Message,
	c *Consumer,
	batcher *storeBatcher[*T],
	identify func(*T) (uuid.UUID, []any),
	kind string,
) error {
	m.DisableAutoResponse()

	var v T
	if err := json.Unmarshal(m.Body, &v); err != nil {
		slog.Error("Failed to unmarshal "+kind+" from NSQ", "error", err, "body", string(m.Body))
		m.Finish()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	siteID, logAttrs := identify(&v)

	store, err := c.resolveStore(ctx, siteID)
	if err != nil {
		slog.Error("Failed to resolve tenant store for "+kind, "error", err, "site_id", siteID)
		m.Requeue(-1)
		return nil
	}

	result, err := batcher.Enqueue(batchItem[*T]{
		message:  m,
		value:    &v,
		store:    store,
		siteID:   siteID,
		logAttrs: logAttrs,
	})
	if err != nil {
		slog.Error("Failed to enqueue "+kind+" for batched persistence", "error", err, "site_id", siteID)
		m.Requeue(-1)
		return nil
	}

	if err := <-result; err != nil {
		slog.Error("Failed to persist "+kind+" batch", "error", err, "site_id", siteID)
		m.Requeue(-1)
		return nil
	}

	m.Finish()
	slog.Debug("Successfully processed "+kind, append([]any{"site_id", siteID}, logAttrs...)...)
	return nil
}

func (c *Consumer) resolveStore(ctx context.Context, siteID uuid.UUID) (*database.Store, error) {
	store, _, err := c.tenantMgr.ResolveSiteStore(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("resolve analytics store for site %s: %w", siteID, err)
	}
	return store, nil
}

func (c *Consumer) publishHitsChanged(hits []*api.Hit) {
	if c.realtime == nil {
		return
	}
	bySite := map[uuid.UUID]siteChange{}
	for _, hit := range hits {
		if hit == nil {
			continue
		}
		change := bySite[hit.SiteID]
		change.count++
		change.noteTimestamp(hit.Timestamp)
		bySite[hit.SiteID] = change
	}
	for siteID, change := range bySite {
		c.realtime.Publish(realtime.Event{
			SiteID:      siteID,
			Kinds:       []string{realtime.KindHits},
			ChangedAt:   time.Now().UTC(),
			BucketStart: change.bucketStart(),
			Counts:      map[string]int{realtime.KindHits: change.count},
		})
	}
}

func (c *Consumer) publishEventsChanged(events []*api.Event) {
	if c.realtime == nil {
		return
	}
	bySite := map[uuid.UUID]siteEventChange{}
	for _, event := range events {
		if event == nil {
			continue
		}
		change := bySite[event.SiteID]
		change.count++
		change.noteTimestamp(event.Timestamp)
		if isEcommerceEvent(event.Name) {
			change.ecommerceCount++
		}
		bySite[event.SiteID] = change
	}
	for siteID, change := range bySite {
		kinds := []string{realtime.KindEvents}
		counts := map[string]int{realtime.KindEvents: change.count}
		if change.ecommerceCount > 0 {
			kinds = append(kinds, realtime.KindEcommerce)
			counts[realtime.KindEcommerce] = change.ecommerceCount
		}
		c.realtime.Publish(realtime.Event{
			SiteID:      siteID,
			Kinds:       kinds,
			ChangedAt:   time.Now().UTC(),
			BucketStart: change.bucketStart(),
			Counts:      counts,
		})
	}
}

func (c *Consumer) publishWebVitalsChanged(vitals []*api.WebVital) {
	if c.realtime == nil {
		return
	}
	bySite := map[uuid.UUID]siteChange{}
	for _, vital := range vitals {
		if vital == nil {
			continue
		}
		change := bySite[vital.SiteID]
		change.count++
		change.noteTimestamp(vital.Timestamp)
		bySite[vital.SiteID] = change
	}
	for siteID, change := range bySite {
		c.realtime.Publish(realtime.Event{
			SiteID:      siteID,
			Kinds:       []string{realtime.KindWebVitals},
			ChangedAt:   time.Now().UTC(),
			BucketStart: change.bucketStart(),
			Counts:      map[string]int{realtime.KindWebVitals: change.count},
		})
	}
}

type siteChange struct {
	count     int
	firstTime time.Time
}

type siteEventChange struct {
	siteChange
	ecommerceCount int
}

func (c *siteChange) noteTimestamp(ts time.Time) {
	if ts.IsZero() {
		return
	}
	if c.firstTime.IsZero() || ts.Before(c.firstTime) {
		c.firstTime = ts
	}
}

func (c siteChange) bucketStart() time.Time {
	if c.firstTime.IsZero() {
		return time.Now().UTC().Truncate(time.Minute)
	}
	return c.firstTime.UTC().Truncate(time.Minute)
}

func isEcommerceEvent(name string) bool {
	switch name {
	case "purchase", "begin_checkout", "view_item", "add_to_cart", "product_viewed", "checkout_started", "order_completed":
		return true
	default:
		return false
	}
}
