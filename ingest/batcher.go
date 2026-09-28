package ingest

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"

	"hitkeep/database"
)

const (
	ingestBatchSize           = 64
	ingestBatchFlushInterval  = 200 * time.Millisecond
	ingestPersistTimeout      = 10 * time.Second
	ingestConsumerConcurrency = 64
)

var errBatcherStopped = errors.New("ingest batcher stopped")

type batchItem[T any] struct {
	message  *nsq.Message
	value    T
	store    *database.Store
	siteID   uuid.UUID
	logAttrs []any
	result   chan error
}

type storeBatcher[T any] struct {
	kind           string
	logger         *slog.Logger
	batchSize      int
	flushInterval  time.Duration
	persistTimeout time.Duration
	persist        func(*database.Store, context.Context, []T) error

	items chan batchItem[T]
	done  chan struct{}
	once  sync.Once
}

func newStoreBatcher[T any](
	kind string,
	logger *slog.Logger,
	batchSize int,
	flushInterval time.Duration,
	persistTimeout time.Duration,
	persist func(*database.Store, context.Context, []T) error,
) *storeBatcher[T] {
	b := &storeBatcher[T]{
		kind:           kind,
		logger:         logger,
		batchSize:      batchSize,
		flushInterval:  flushInterval,
		persistTimeout: persistTimeout,
		persist:        persist,
		items:          make(chan batchItem[T], batchSize*2),
		done:           make(chan struct{}),
	}
	go b.run()
	return b
}

func (b *storeBatcher[T]) Enqueue(item batchItem[T]) (<-chan error, error) {
	select {
	case <-b.done:
		return nil, errBatcherStopped
	default:
	}

	item.result = make(chan error, 1)
	b.items <- item
	return item.result, nil
}

func (b *storeBatcher[T]) Stop() {
	b.once.Do(func() {
		close(b.items)
		<-b.done
	})
}

func (b *storeBatcher[T]) run() {
	defer close(b.done)

	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	var pending []batchItem[T]
	for {
		select {
		case item, ok := <-b.items:
			if !ok {
				b.flush(pending)
				return
			}
			pending = append(pending, item)
			if len(pending) >= b.batchSize {
				b.flush(pending)
				pending = nil
			}
		case <-ticker.C:
			if len(pending) == 0 {
				continue
			}
			b.flush(pending)
			pending = nil
		}
	}
}

func (b *storeBatcher[T]) flush(items []batchItem[T]) {
	if len(items) == 0 {
		return
	}

	grouped := make(map[*database.Store][]batchItem[T], len(items))
	for _, item := range items {
		grouped[item.store] = append(grouped[item.store], item)
	}

	for store, group := range grouped {
		values := make([]T, len(group))
		for i := range group {
			values[i] = group[i].value
		}

		ctx, cancel := context.WithTimeout(context.Background(), b.persistTimeout)
		err := b.persist(store, ctx, values)
		cancel()

		if err != nil {
			b.logger.Error("Failed to flush batched ingest records", "kind", b.kind, "count", len(group), "error", err)
		} else {
			b.logger.Debug("Flushed batched ingest records", "kind", b.kind, "count", len(group))
		}

		for _, item := range group {
			item.result <- err
			close(item.result)
		}
	}
}
