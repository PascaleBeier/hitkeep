package webhookdispatcher

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"hitkeep/internal/config"
	"hitkeep/internal/database"
)

type Sweeper struct {
	store    *database.Store
	producer Producer
	config   config.Config
	mu       sync.Mutex
	lastGC   time.Time
}

func NewSweeper(store *database.Store, producer Producer, conf config.Config) *Sweeper {
	return &Sweeper{store: store, producer: producer, config: conf}
}

func (s *Sweeper) RunOnce(ctx context.Context, now time.Time) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("webhook sweeper store is not configured")
	}
	timeout := time.Duration(s.config.WebhookDeliveryTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	staleAfter := max(2*timeout, time.Minute)
	if _, err := s.store.RecoverStaleWebhookDeliveries(ctx, now.Add(-staleAfter), now); err != nil {
		return err
	}

	jobs, err := s.store.ListDueWebhookDeliveryJobs(ctx, now, 500)
	if err != nil {
		return err
	}
	for _, job := range jobs {
		if s.producer == nil {
			continue
		}
		body, err := json.Marshal(WebhookDeliveryMessage{DeliveryID: job.DeliveryID})
		if err != nil {
			return fmt.Errorf("marshal recovered webhook delivery: %w", err)
		}
		if err := s.producer.Publish(Topic, body); err != nil {
			slog.Warn("Webhook delivery remains pending after sweep publish failure", "error", err, "delivery_id", job.DeliveryID)
		}
	}

	if s.retentionDue(now) {
		retentionDays := s.config.WebhookRetentionDays
		if retentionDays <= 0 {
			retentionDays = 30
		}
		if _, err := s.store.DeleteWebhookHistoryBefore(ctx, now.AddDate(0, 0, -retentionDays)); err != nil {
			return err
		}
		s.markRetentionRun(now)
	}
	return nil
}

func (s *Sweeper) retentionDue(now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastGC.IsZero() || now.Sub(s.lastGC) >= 24*time.Hour
}

func (s *Sweeper) markRetentionRun(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastGC = now
}

func (s *Sweeper) Start(ctx context.Context) {
	interval := time.Duration(s.config.WebhookSweepSeconds) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if err := s.RunOnce(ctx, time.Now().UTC()); err != nil {
		slog.Error("Webhook recovery sweep failed", "error", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.RunOnce(ctx, now.UTC()); err != nil {
				slog.Error("Webhook recovery sweep failed", "error", err)
			}
		}
	}
}
