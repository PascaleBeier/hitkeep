package imports

import (
	"context"
	"log/slog"
	"sync"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

type importRequest struct {
	siteID   uuid.UUID
	importID uuid.UUID
}

type importRunner struct {
	h     *handler
	queue chan importRequest
	once  sync.Once
}

func newImportRunner(h *handler) *importRunner {
	return &importRunner{
		h:     h,
		queue: make(chan importRequest, 128),
	}
}

func (r *importRunner) Start(ctx context.Context) {
	if r == nil {
		return
	}
	r.once.Do(func() {
		go r.loop(ctx)
		go r.recoverRunnable(ctx)
	})
}

func (r *importRunner) Enqueue(siteID, importID uuid.UUID) {
	if r == nil {
		return
	}
	req := importRequest{siteID: siteID, importID: importID}
	select {
	case r.queue <- req:
	default:
		go func() {
			r.queue <- req
		}()
	}
}

func (r *importRunner) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-r.queue:
			r.h.runImport(req.siteID, req.importID)
		}
	}
}

func (r *importRunner) recoverRunnable(ctx context.Context) {
	if r == nil || r.h == nil || r.h.ctx == nil || r.h.ctx.Store == nil {
		return
	}
	jobs, err := r.h.ctx.Store.ListRunnableImports(ctx)
	if err != nil {
		slog.Error("Failed to recover import jobs", "error", err)
		return
	}
	for _, job := range jobs {
		if _, ok := r.h.registry.Provider(job.Provider); !ok {
			_ = r.h.ctx.Store.MarkImportFailed(ctx, job.SiteID, job.ID, "unknown importer")
			r.h.appendImportAudit(ctx, nil, job.SiteID, job.ID, importActorID(&job), job.Provider, "import.failed", "failure", "unknown importer")
			continue
		}
		if _, err := r.h.sourceSet(ctx, job.SiteID, job.ID, false); err != nil {
			_ = r.h.ctx.Store.MarkImportFailed(ctx, job.SiteID, job.ID, err.Error())
			r.h.appendImportAudit(ctx, nil, job.SiteID, job.ID, importActorID(&job), job.Provider, "import.failed", "failure", err.Error())
			continue
		}
		if job.Status == database.ImportStatusRunning {
			if err := r.h.ctx.Store.MarkImportQueued(ctx, job.SiteID, job.ID); err != nil {
				slog.Error("Failed to requeue interrupted import", "error", err, "site_id", job.SiteID, "import_id", job.ID)
				continue
			}
			r.h.appendImportAudit(ctx, nil, job.SiteID, job.ID, importActorID(&job), job.Provider, "import.requeued", "success", "Interrupted import requeued")
		}
		r.Enqueue(job.SiteID, job.ID)
	}
}
