package webhookdispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"hitkeep/config"
	"hitkeep/internal/database"
	json "hitkeep/jsonapi"
)

type Sweeper struct {
	store    *database.Store
	producer Producer
	config   config.Config
	logger   *slog.Logger
	mu       sync.Mutex
	lastGC   time.Time
}

func NewSweeper(store *database.Store, producer Producer, conf config.Config, logger *slog.Logger) *Sweeper {
	if logger == nil {
		panic("webhookdispatcher: logger is required")
	}
	return &Sweeper{store: store, producer: producer, config: conf, logger: logger}
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
	if _, err := s.store.RecoverStagedSiteDeletionWebhookEvents(ctx, now); err != nil {
		return err
	}
	if _, err := s.store.DeleteAbandonedStagedSiteDeletionWebhookEvents(ctx, now.Add(-24*time.Hour)); err != nil {
		return err
	}

	sweepInterval := time.Duration(s.config.WebhookSweepSeconds) * time.Second
	if sweepInterval <= 0 {
		sweepInterval = 30 * time.Second
	}
	queueLease := max(2*sweepInterval, time.Minute)
	jobs, err := s.store.ListDispatchableWebhookDeliveryJobs(ctx, now, now.Add(-queueLease), 500)
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
		marked := true
		if err := s.store.MarkWebhookDeliveryQueued(ctx, job.DeliveryID, now); err != nil {
			marked = false
			s.logger.Warn("Webhook recovery publish could not acquire queue marker", "error", err, "delivery_id", job.DeliveryID)
		}
		if err := s.producer.Publish(Topic, body); err != nil {
			s.logger.Warn("Webhook delivery remains pending after sweep publish failure", "error", err, "delivery_id", job.DeliveryID)
			if marked {
				if clearErr := s.store.ClearWebhookDeliveryQueued(ctx, job.DeliveryID, time.Now().UTC()); clearErr != nil {
					s.logger.Warn("Failed to release webhook recovery queue marker", "error", clearErr, "delivery_id", job.DeliveryID)
				}
			}
			continue
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
		s.logger.Error("Webhook recovery sweep failed", "error", err)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.RunOnce(ctx, now.UTC()); err != nil {
				s.logger.Error("Webhook recovery sweep failed", "error", err)
			}
		}
	}
}
