package webhookdispatcher

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"

	"hitkeep/config"
	"hitkeep/hklog"
	"hitkeep/internal/database"
	json "hitkeep/jsonapi"
)

const Channel = "dispatcher"

type Worker struct {
	store      *database.Store
	producer   Producer
	config     config.Config
	dispatcher *Dispatcher
	logger     *slog.Logger
	logLevel   slog.Level
	consumer   *nsq.Consumer
	limits     sync.Map
}

func NewWorker(store *database.Store, producer Producer, conf config.Config, logger *slog.Logger, logLevel slog.Level) *Worker {
	if logger == nil {
		panic("webhookdispatcher: logger is required")
	}
	return &Worker{
		store:      store,
		producer:   producer,
		config:     conf,
		dispatcher: NewDispatcher(store, conf),
		logger:     logger,
		logLevel:   logLevel,
	}
}

func (w *Worker) Connect(ctx context.Context, addr string) error {
	if w == nil || w.store == nil {
		return fmt.Errorf("webhook worker store is not configured")
	}
	consumerConfig := nsq.NewConfig()
	consumerConfig.MaxAttempts = 10
	consumer, err := nsq.NewConsumer(Topic, Channel, consumerConfig)
	if err != nil {
		return fmt.Errorf("create webhook delivery consumer: %w", err)
	}
	consumer.SetLogger(hklog.GoNSQLogger{Logger: w.logger}, hklog.NSQGoLevel(w.logLevel))
	concurrency := w.config.WebhookDeliveryConcurrency
	if concurrency <= 0 {
		concurrency = 8
	}
	consumer.AddConcurrentHandlers(nsq.HandlerFunc(w.handleMessage), concurrency)
	if err := consumer.ConnectToNSQD(addr); err != nil {
		consumer.Stop()
		return fmt.Errorf("connect webhook delivery consumer: %w", err)
	}
	w.consumer = consumer
	go NewSweeper(w.store, w.producer, w.config, w.logger).Start(ctx)
	return nil
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}
	if w.consumer != nil {
		w.consumer.Stop()
		<-w.consumer.StopChan
	}
	if w.dispatcher != nil && w.dispatcher.httpClient != nil {
		w.dispatcher.httpClient.CloseIdleConnections()
	}
}

func (w *Worker) handleMessage(message *nsq.Message) error {
	var payload WebhookDeliveryMessage
	if err := json.Unmarshal(message.Body, &payload); err != nil {
		return fmt.Errorf("decode webhook delivery message: %w", err)
	}
	if payload.DeliveryID == uuid.Nil {
		return fmt.Errorf("decode webhook delivery message: delivery_id is required")
	}
	delivery, err := w.store.GetWebhookDelivery(context.Background(), payload.DeliveryID)
	if err != nil || delivery == nil {
		return err
	}
	release, ok := w.tryAcquire(delivery.WebhookID)
	if !ok {
		message.DisableAutoResponse()
		message.RequeueWithoutBackoff(250 * time.Millisecond)
		return nil
	}
	defer release()
	return w.dispatcher.Dispatch(context.Background(), payload.DeliveryID)
}

func (w *Worker) tryAcquire(webhookID uuid.UUID) (func(), bool) {
	limit := w.config.WebhookPerEndpointConcurrency
	if limit <= 0 {
		limit = 1
	}
	value, _ := w.limits.LoadOrStore(webhookID, make(chan struct{}, limit))
	semaphore := value.(chan struct{})
	select {
	case semaphore <- struct{}{}:
		return func() { <-semaphore }, true
	default:
		return nil, false
	}
}
