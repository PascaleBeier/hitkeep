package imports

import (
	"context"
	"errors"
	"sync"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/webhooks"
)

var (
	errImportRunnerNotStarted = errors.New("import runner is not started")
	errImportRunnerStopped    = errors.New("import runner is stopped")
	errImportQueueFull        = errors.New("import queue is full")
)

type importRequest struct {
	siteID   uuid.UUID
	importID uuid.UUID
}

type importRunner struct {
	h      *handler
	queue  chan importRequest
	once   sync.Once
	stop   sync.Once
	wg     sync.WaitGroup
	done   chan struct{}
	mu     sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

func newImportRunner(h *handler) *importRunner {
	return &importRunner{
		h:     h,
		queue: make(chan importRequest, 128),
		done:  make(chan struct{}),
	}
}

func (r *importRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.once.Do(func() {
		// The runner retains cancel and invokes it from Stop during server shutdown.
		runCtx, cancel := context.WithCancel(ctx) // #nosec G118 -- cancellation is owned by Stop.
		r.mu.Lock()
		r.ctx = runCtx
		r.cancel = cancel
		r.mu.Unlock()

		r.wg.Add(2)
		go func() {
			defer r.wg.Done()
			r.loop(runCtx)
		}()
		go func() {
			defer r.wg.Done()
			r.recoverRunnable(runCtx)
		}()
		go func() {
			r.wg.Wait()
			close(r.done)
		}()
	})
}

func (r *importRunner) Enqueue(ctx context.Context, siteID, importID uuid.UUID) error {
	if r == nil {
		return errImportRunnerNotStarted
	}
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.RLock()
	runCtx := r.ctx
	r.mu.RUnlock()
	if runCtx == nil {
		return errImportRunnerNotStarted
	}
	if err := runCtx.Err(); err != nil {
		return errImportRunnerStopped
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	req := importRequest{siteID: siteID, importID: importID}
	select {
	case <-runCtx.Done():
		return errImportRunnerStopped
	case <-ctx.Done():
		return ctx.Err()
	case r.queue <- req:
		return nil
	}
}

func (r *importRunner) TryEnqueue(siteID, importID uuid.UUID) error {
	if r == nil {
		return errImportRunnerNotStarted
	}
	r.mu.RLock()
	runCtx := r.ctx
	r.mu.RUnlock()
	if runCtx == nil {
		return errImportRunnerNotStarted
	}
	if runCtx.Err() != nil {
		return errImportRunnerStopped
	}
	select {
	case r.queue <- importRequest{siteID: siteID, importID: importID}:
		return nil
	default:
		return errImportQueueFull
	}
}

func (r *importRunner) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	cancel := r.cancel
	done := r.done
	r.mu.RUnlock()
	if cancel == nil {
		return nil
	}
	r.stop.Do(cancel)
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *importRunner) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-r.queue:
			if ctx.Err() != nil {
				return
			}
			r.h.runImportContext(ctx, req.siteID, req.importID)
		}
	}
}

func (r *importRunner) recoverRunnable(ctx context.Context) {
	if r == nil || r.h == nil || r.h.ctx == nil || r.h.ctx.Store == nil {
		return
	}
	ctx = shared.WithLogger(ctx, r.h.ctx.Logger)
	jobs, err := r.h.ctx.Store.ListRunnableImports(ctx)
	if err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to recover import jobs", "error", err)
		return
	}
	for _, job := range jobs {
		if _, ok := r.h.registry.Provider(job.Provider); !ok {
			_ = r.h.ctx.Store.MarkImportFailed(ctx, job.SiteID, job.ID, "unknown importer")
			r.h.appendImportAudit(ctx, nil, job.SiteID, job.ID, importActorID(&job), job.Provider, "import.failed", "failure", "unknown importer")
			r.h.ctx.EmitWebhookEvent(ctx, webhooks.Event{Type: webhooks.EventImportFailed, SiteID: &job.SiteID, Data: map[string]any{
				"site_id": job.SiteID.String(), "import_id": job.ID.String(), "provider": job.Provider,
			}})
			continue
		}
		if _, err := r.h.sourceSet(ctx, job.SiteID, job.ID, false); err != nil {
			_ = r.h.ctx.Store.MarkImportFailed(ctx, job.SiteID, job.ID, err.Error())
			r.h.appendImportAudit(ctx, nil, job.SiteID, job.ID, importActorID(&job), job.Provider, "import.failed", "failure", err.Error())
			r.h.ctx.EmitWebhookEvent(ctx, webhooks.Event{Type: webhooks.EventImportFailed, SiteID: &job.SiteID, Data: map[string]any{
				"site_id": job.SiteID.String(), "import_id": job.ID.String(), "provider": job.Provider,
			}})
			continue
		}
		if job.Status == database.ImportStatusRunning {
			if err := r.h.ctx.Store.MarkImportQueued(ctx, job.SiteID, job.ID); err != nil {
				shared.LoggerFromContext(ctx).Error("Failed to requeue interrupted import", "error", err, "site_id", job.SiteID, "import_id", job.ID)
				continue
			}
			r.h.appendImportAudit(ctx, nil, job.SiteID, job.ID, importActorID(&job), job.Provider, "import.requeued", "success", "Interrupted import requeued")
		}
		if err := r.Enqueue(ctx, job.SiteID, job.ID); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				shared.LoggerFromContext(ctx).Error("Failed to enqueue recovered import", "error", err, "site_id", job.SiteID, "import_id", job.ID)
			}
			return
		}
	}
}
