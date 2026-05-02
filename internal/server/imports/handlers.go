package imports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/database"
	"hitkeep/internal/importables"
	"hitkeep/internal/server/shared"
)

const defaultImportChunkSize = 8 << 20

type handler struct {
	ctx      *shared.Context
	registry *importables.Registry
	runner   *importRunner
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{
		ctx: ctx,
		registry: importables.NewRegistry(
			importables.NewPlausibleProvider(),
			importables.NewSimpleAnalyticsProvider(),
		),
	}
	h.runner = newImportRunner(h)
	h.runner.Start(context.Background())

	requireImportAccess := ctx.RequireSiteOrInstancePermission(authcore.PermSiteManageData, authcore.PermInstanceManageImports)
	mux.HandleFunc("GET /api/sites/{id}/importers", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, requireImportAccess(h.handleListImporters())))
	mux.HandleFunc("POST /api/sites/{id}/imports/{provider}/uploads", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, requireImportAccess(h.handleCreateUpload())))
	mux.HandleFunc("PUT /api/sites/{id}/imports/uploads/{importID}/files/{fileID}/chunks", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, requireImportAccess(h.handleUploadChunk())))
	mux.HandleFunc("POST /api/sites/{id}/imports/uploads/{importID}/validate", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, requireImportAccess(h.handleValidateUpload())))
	mux.HandleFunc("GET /api/sites/{id}/imports/{importID}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, requireImportAccess(h.handleGetImport())))
	mux.HandleFunc("POST /api/sites/{id}/imports/{importID}/start", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, requireImportAccess(h.handleStartImport())))
	mux.HandleFunc("GET /api/sites/{id}/imports", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, requireImportAccess(h.handleListImports())))
	mux.HandleFunc("DELETE /api/sites/{id}/imports/{importID}", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		AllowAPIKey: true,
		RateLimiter: ctx.ApiLimiter,
	}, requireImportAccess(h.handleDeleteImport())))
}

func (h *handler) appendImportAudit(ctx context.Context, r *http.Request, siteID, importID, actorID uuid.UUID, provider, action, outcome, details string) {
	if h.ctx == nil || h.ctx.Store == nil {
		return
	}
	if ctx == nil {
		if r != nil {
			ctx = r.Context()
		} else {
			ctx = context.Background()
		}
	}
	teamID, err := h.ctx.Store.GetSiteTenantID(ctx, siteID)
	if err != nil {
		slog.Warn("Failed to resolve team for import audit", "error", err, "site_id", siteID, "import_id", importID, "action", action)
		return
	}
	siteLabel := siteID.String()
	if site, err := h.ctx.Store.GetSiteByID(ctx, siteID); err == nil && site != nil && strings.TrimSpace(site.Domain) != "" {
		siteLabel = site.Domain
	}
	if strings.TrimSpace(details) == "" {
		details = fmt.Sprintf("Import %s for %s", importID, siteLabel)
	}
	label := siteLabel
	if strings.TrimSpace(provider) != "" {
		label = fmt.Sprintf("%s import for %s", provider, siteLabel)
	}
	h.ctx.AppendAuditEvent(ctx, r, shared.AuditEvent{
		ActorID:     actorID,
		TeamID:      teamID,
		Action:      action,
		TargetType:  "import",
		TargetID:    importID.String(),
		TargetLabel: label,
		Outcome:     outcome,
		Details:     details,
	})
}

func importActorID(job *api.ImportJob) uuid.UUID {
	if job != nil && job.CreatedBy != nil {
		return *job.CreatedBy
	}
	return uuid.Nil
}

func (h *handler) handleListImporters() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, h.registry.Descriptors())
	}
}

func (h *handler) handleCreateUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		providerKey := strings.TrimSpace(r.PathValue("provider"))
		if _, ok := h.registry.Provider(providerKey); !ok {
			http.Error(w, "Unknown importer", http.StatusBadRequest)
			return
		}

		var req api.ImportUploadCreateRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
		if err := decoder.Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		if len(req.Files) == 0 {
			http.Error(w, "At least one file is required", http.StatusBadRequest)
			return
		}

		maxBytes := int64(h.ctx.Config.ImportMaxStageBytes)
		if maxBytes <= 0 {
			maxBytes = 100 * 1024 * 1024 * 1024
		}
		files := make([]database.ImportFileCreate, 0, len(req.Files))
		var total int64
		for _, input := range req.Files {
			filename := sanitizeUploadFilename(input.Filename)
			if filename == "" {
				http.Error(w, "Invalid filename", http.StatusBadRequest)
				return
			}
			ext := strings.ToLower(filepath.Ext(filename))
			if ext != ".zip" && ext != ".csv" {
				http.Error(w, "Only .zip and .csv files are supported", http.StatusBadRequest)
				return
			}
			if input.SizeBytes <= 0 {
				http.Error(w, "File size must be greater than zero", http.StatusBadRequest)
				return
			}
			sha256Sum := strings.ToLower(strings.TrimSpace(input.SHA256))
			if sha256Sum != "" && !sha256HexPattern.MatchString(sha256Sum) {
				http.Error(w, "Invalid sha256 checksum", http.StatusBadRequest)
				return
			}
			total += input.SizeBytes
			if total > maxBytes {
				http.Error(w, "Import exceeds maximum staged size", http.StatusRequestEntityTooLarge)
				return
			}
			fileID := uuid.New()
			files = append(files, database.ImportFileCreate{
				ID:           fileID,
				Filename:     filename,
				RelativePath: filepath.Join("imports", siteID.String(), fileID.String()+"-"+filename),
				SizeBytes:    input.SizeBytes,
				SHA256:       sha256Sum,
			})
		}

		actorID := shared.GetUserIDFromContext(r)
		job, err := h.ctx.Store.CreateSiteImportUpload(r.Context(), siteID, actorID, providerKey, files)
		if err != nil {
			slog.Error("Failed to create import upload", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		// #nosec G703 -- siteID is parsed as a UUID and this path stays under HitKeep's configured data directory.
		if err := os.MkdirAll(filepath.Join(h.ctx.Config.DataPath, "imports", siteID.String()), 0755); err != nil {
			slog.Error("Failed to create import staging directory", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendImportAudit(r.Context(), r, siteID, job.ID, actorID, job.Provider, "import.upload_created", "success", fmt.Sprintf("%s import upload created with %d file(s)", job.Provider, len(job.Files)))

		writeJSON(w, api.ImportUploadCreateResponse{
			ImportID:  job.ID,
			Provider:  job.Provider,
			Status:    job.Status,
			ChunkSize: defaultImportChunkSize,
			Files:     job.Files,
		})
	}
}

func (h *handler) handleUploadChunk() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		importID, ok := parseUUIDPath(w, r, "importID", "Invalid import_id")
		if !ok {
			return
		}
		fileID, ok := parseUUIDPath(w, r, "fileID", "Invalid file_id")
		if !ok {
			return
		}
		job, err := h.ctx.Store.GetSiteImport(r.Context(), siteID, importID)
		if err != nil || job == nil {
			http.Error(w, "Import not found", http.StatusNotFound)
			return
		}
		if job.Status != database.ImportStatusUploading {
			http.Error(w, "Import is not accepting uploads", http.StatusConflict)
			return
		}
		file, err := h.ctx.Store.GetImportFile(r.Context(), importID, fileID)
		if err != nil {
			http.Error(w, "Import file not found", http.StatusNotFound)
			return
		}
		offset, err := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("offset")), 10, 64)
		if err != nil || offset < 0 || offset > file.SizeBytes {
			http.Error(w, "Invalid offset", http.StatusBadRequest)
			return
		}
		if offset > file.BytesReceived {
			http.Error(w, "Chunk offset is beyond uploaded range", http.StatusConflict)
			return
		}
		if offset < file.BytesReceived {
			replayLimit := file.BytesReceived - offset
			limited := &io.LimitedReader{R: r.Body, N: replayLimit + 1}
			read, err := io.Copy(io.Discard, limited)
			if err != nil {
				slog.Error("Failed to read duplicate import chunk", "error", err, "import_id", importID, "file_id", fileID)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
			if read > replayLimit {
				http.Error(w, "Chunk overlaps uploaded boundary", http.StatusConflict)
				return
			}
			writeJSON(w, api.ImportChunkResponse{ImportID: importID, FileID: fileID, BytesReceived: file.BytesReceived, Complete: file.BytesReceived >= file.SizeBytes})
			return
		}
		remaining := file.SizeBytes - offset
		if remaining <= 0 {
			writeJSON(w, api.ImportChunkResponse{ImportID: importID, FileID: fileID, BytesReceived: file.BytesReceived, Complete: true})
			return
		}

		path := h.stagedPath(file.RelativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			slog.Error("Failed to create import file directory", "error", err, "path", path)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			slog.Error("Failed to open staged import file", "error", err, "path", path)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		defer out.Close()
		if _, err := out.Seek(offset, io.SeekStart); err != nil {
			slog.Error("Failed to seek staged import file", "error", err, "path", path)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		limited := &io.LimitedReader{R: r.Body, N: remaining + 1}
		written, err := io.Copy(out, limited)
		if err != nil {
			slog.Error("Failed to write import chunk", "error", err, "path", path)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if written > remaining {
			_ = out.Truncate(file.SizeBytes)
			http.Error(w, "Chunk exceeds declared file size", http.StatusRequestEntityTooLarge)
			return
		}

		bytesReceived := offset + written
		if err := h.ctx.Store.UpdateImportFileProgress(r.Context(), importID, fileID, bytesReceived, ""); err != nil {
			slog.Error("Failed to update import upload progress", "error", err, "import_id", importID, "file_id", fileID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if bytesReceived >= file.SizeBytes && file.Status != database.ImportFileStatusUploaded {
			actorID := shared.GetUserIDFromContext(r)
			if actorID == uuid.Nil {
				actorID = importActorID(job)
			}
			h.appendImportAudit(r.Context(), r, siteID, importID, actorID, job.Provider, "import.file_uploaded", "success", fmt.Sprintf("Import file %s uploaded", file.Filename))
		}
		writeJSON(w, api.ImportChunkResponse{ImportID: importID, FileID: fileID, BytesReceived: bytesReceived, Complete: bytesReceived >= file.SizeBytes})
	}
}

func (h *handler) handleValidateUpload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		importID, ok := parseUUIDPath(w, r, "importID", "Invalid import_id")
		if !ok {
			return
		}
		job, err := h.ctx.Store.GetSiteImport(r.Context(), siteID, importID)
		if err != nil || job == nil {
			http.Error(w, "Import not found", http.StatusNotFound)
			return
		}
		if job.Status != database.ImportStatusUploading && job.Status != database.ImportStatusValidationFailed {
			http.Error(w, "Import cannot be validated", http.StatusConflict)
			return
		}
		provider, ok := h.registry.Provider(job.Provider)
		if !ok {
			http.Error(w, "Unknown importer", http.StatusBadRequest)
			return
		}
		actorID := shared.GetUserIDFromContext(r)
		if actorID == uuid.Nil {
			actorID = importActorID(job)
		}
		appendValidationFailed := func(message string) {
			h.appendImportAudit(r.Context(), r, siteID, importID, actorID, job.Provider, "import.validation_failed", "failure", message)
		}
		if err := h.ctx.Store.MarkImportValidating(r.Context(), siteID, importID); err != nil {
			slog.Error("Failed to mark import validating", "error", err, "import_id", importID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendImportAudit(r.Context(), r, siteID, importID, actorID, job.Provider, "import.validation_started", "success", fmt.Sprintf("%s import validation started", job.Provider))

		sourceSet, err := h.sourceSet(r.Context(), siteID, importID, true)
		if err != nil {
			_ = h.ctx.Store.MarkImportValidationFailed(r.Context(), siteID, importID, err.Error())
			appendValidationFailed(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		duplicate, err := h.ctx.Store.CompletedImportExistsForSourceHash(r.Context(), siteID, job.Provider, sourceSet.SourceHash, importID)
		if err != nil {
			slog.Error("Failed to check duplicate import source", "error", err, "import_id", importID)
			_ = h.ctx.Store.MarkImportValidationFailed(r.Context(), siteID, importID, "could not check duplicate imports")
			appendValidationFailed("could not check duplicate imports")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if duplicate {
			errMsg := "this source has already been imported"
			_ = h.ctx.Store.MarkImportValidationFailed(r.Context(), siteID, importID, errMsg)
			appendValidationFailed(errMsg)
			http.Error(w, errMsg, http.StatusConflict)
			return
		}
		manifest, err := provider.Validate(r.Context(), sourceSet)
		if err != nil {
			_ = h.ctx.Store.MarkImportValidationFailed(r.Context(), siteID, importID, err.Error())
			appendValidationFailed(err.Error())
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		manifest.SourceHash = sourceSet.SourceHash
		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			_ = h.ctx.Store.MarkImportValidationFailed(r.Context(), siteID, importID, "could not resolve analytics store")
			appendValidationFailed("could not resolve analytics store")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if _, err := analyticsStore.AnnotateImportManifestOverlap(r.Context(), siteID, manifest); err != nil {
			slog.Error("Failed to calculate import overlap", "error", err, "site_id", siteID, "import_id", importID)
			_ = h.ctx.Store.MarkImportValidationFailed(r.Context(), siteID, importID, "could not calculate native overlap")
			appendValidationFailed("could not calculate native overlap")
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if err := h.ctx.Store.MarkImportValidated(r.Context(), siteID, importID, sourceSet.SourceHash, manifest); err != nil {
			slog.Error("Failed to save import validation", "error", err, "import_id", importID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendImportAudit(r.Context(), r, siteID, importID, actorID, job.Provider, "import.validated", "success", fmt.Sprintf("%s import validated with %d accepted row(s)", job.Provider, manifest.RowsAccepted))
		job, _ = h.ctx.Store.GetSiteImport(r.Context(), siteID, importID)
		writeJSON(w, job)
	}
}

func (h *handler) handleGetImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		importID, ok := parseUUIDPath(w, r, "importID", "Invalid import_id")
		if !ok {
			return
		}
		job, err := h.ctx.Store.GetSiteImport(r.Context(), siteID, importID)
		if err != nil || job == nil {
			http.Error(w, "Import not found", http.StatusNotFound)
			return
		}
		writeJSON(w, job)
	}
}

func (h *handler) handleListImports() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		imports, err := h.ctx.Store.ListSiteImports(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to list imports", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, api.ImportListResponse{Imports: imports})
	}
}

func (h *handler) handleStartImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		importID, ok := parseUUIDPath(w, r, "importID", "Invalid import_id")
		if !ok {
			return
		}
		job, err := h.ctx.Store.GetSiteImport(r.Context(), siteID, importID)
		if err != nil || job == nil {
			http.Error(w, "Import not found", http.StatusNotFound)
			return
		}
		if job.Status != database.ImportStatusValidated {
			http.Error(w, "Import must be validated before it can start", http.StatusConflict)
			return
		}
		if _, ok := h.registry.Provider(job.Provider); !ok {
			http.Error(w, "Unknown importer", http.StatusBadRequest)
			return
		}
		if h.runner == nil {
			http.Error(w, "Import runner is not available", http.StatusServiceUnavailable)
			return
		}
		if err := h.ctx.Store.MarkImportQueued(r.Context(), siteID, importID); err != nil {
			slog.Error("Failed to queue import", "error", err, "import_id", importID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		actorID := shared.GetUserIDFromContext(r)
		if actorID == uuid.Nil {
			actorID = importActorID(job)
		}
		h.appendImportAudit(r.Context(), r, siteID, importID, actorID, job.Provider, "import.queued", "success", fmt.Sprintf("%s import queued", job.Provider))

		h.runner.Enqueue(siteID, importID)

		job, _ = h.ctx.Store.GetSiteImport(r.Context(), siteID, importID)
		writeJSON(w, job)
	}
}

func (h *handler) handleDeleteImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		importID, ok := parseUUIDPath(w, r, "importID", "Invalid import_id")
		if !ok {
			return
		}
		job, err := h.ctx.Store.GetSiteImport(r.Context(), siteID, importID)
		if err != nil || job == nil {
			http.Error(w, "Import not found", http.StatusNotFound)
			return
		}
		if job.Status == database.ImportStatusRunning || job.Status == database.ImportStatusQueued || job.Status == database.ImportStatusValidating {
			http.Error(w, "Import is still running", http.StatusConflict)
			return
		}
		analyticsStore, err := h.ctx.AnalyticsStore(r.Context(), siteID)
		if err != nil {
			slog.Error("Failed to resolve analytics store", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		deletedRows, err := analyticsStore.DeleteImportedDataForImport(r.Context(), siteID, importID)
		if err != nil {
			slog.Error("Failed to delete imported analytics", "error", err, "site_id", siteID, "import_id", importID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		actorID := shared.GetUserIDFromContext(r)
		if actorID == uuid.Nil {
			actorID = importActorID(job)
		}
		if deletedRows > 0 {
			h.appendImportAudit(r.Context(), r, siteID, importID, actorID, job.Provider, "import.data_cleared", "success", fmt.Sprintf("Cleared %d imported row(s)", deletedRows))
		}
		if err := h.ctx.Store.MarkImportDeleted(r.Context(), siteID, importID); err != nil {
			slog.Error("Failed to mark import deleted", "error", err, "import_id", importID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.appendImportAudit(r.Context(), r, siteID, importID, actorID, job.Provider, "import.deleted", "success", fmt.Sprintf("%s import deleted", job.Provider))
		h.cleanupStagedFiles(importID)
		writeJSON(w, map[string]string{"status": "deleted"})
	}
}

func (h *handler) runImport(siteID, importID uuid.UUID) {
	ctx := contextWithoutCancel()
	job, err := h.ctx.Store.GetSiteImport(ctx, siteID, importID)
	if err != nil || job == nil {
		slog.Error("Cannot start missing import", "error", err, "site_id", siteID, "import_id", importID)
		return
	}
	if job.Status != database.ImportStatusValidated &&
		job.Status != database.ImportStatusQueued &&
		job.Status != database.ImportStatusRunning {
		return
	}
	actorID := importActorID(job)
	markFailed := func(message string) {
		_ = h.ctx.Store.MarkImportFailed(ctx, siteID, importID, message)
		h.appendImportAudit(ctx, nil, siteID, importID, actorID, job.Provider, "import.failed", "failure", message)
	}
	provider, ok := h.registry.Provider(job.Provider)
	if !ok {
		markFailed("unknown importer")
		return
	}
	if err := h.ctx.Store.MarkImportRunning(ctx, siteID, importID); err != nil {
		slog.Error("Failed to mark import running", "error", err, "import_id", importID)
		return
	}
	h.appendImportAudit(ctx, nil, siteID, importID, actorID, job.Provider, "import.started", "success", fmt.Sprintf("%s import started", job.Provider))
	if job.SourceHash == "" {
		markFailed("import must be validated before it can start")
		return
	}
	sourceSet, err := h.sourceSet(ctx, siteID, importID, true)
	if err != nil {
		markFailed(err.Error())
		return
	}
	if sourceSet.SourceHash != job.SourceHash {
		markFailed("staged files changed after validation")
		return
	}
	duplicate, err := h.ctx.Store.CompletedImportExistsForSourceHash(ctx, siteID, job.Provider, sourceSet.SourceHash, importID)
	if err != nil {
		markFailed("could not check duplicate imports")
		return
	}
	if duplicate {
		markFailed("this source has already been imported")
		return
	}
	analyticsStore, err := h.ctx.AnalyticsStore(ctx, siteID)
	if err != nil {
		markFailed("could not resolve analytics store")
		return
	}
	deletedRows, err := analyticsStore.DeleteImportedDataForImport(ctx, siteID, importID)
	if err != nil {
		markFailed("could not clear previous imported rows")
		return
	}
	if deletedRows > 0 {
		h.appendImportAudit(ctx, nil, siteID, importID, actorID, job.Provider, "import.data_cleared", "success", fmt.Sprintf("Cleared %d imported row(s)", deletedRows))
	}
	var overlap *database.ImportOverlapPlan
	if job.Manifest != nil {
		overlap, err = analyticsStore.AnnotateImportManifestOverlap(ctx, siteID, job.Manifest)
		if err != nil {
			markFailed("could not calculate native overlap")
			return
		}
	}
	sink, err := database.NewImportedDataSinkWithOptions(ctx, analyticsStore, siteID, importID, database.ImportedDataSinkOptions{Overlap: overlap})
	if err != nil {
		markFailed(err.Error())
		return
	}
	manifest, err := provider.Import(ctx, sourceSet, sink)
	if err != nil {
		sink.Abort()
		deletedRows, _ := analyticsStore.DeleteImportedDataForImport(ctx, siteID, importID)
		if deletedRows > 0 {
			h.appendImportAudit(ctx, nil, siteID, importID, actorID, job.Provider, "import.data_cleared", "success", fmt.Sprintf("Cleared %d imported row(s) after failed import", deletedRows))
		}
		markFailed(err.Error())
		return
	}
	manifest.SourceHash = sourceSet.SourceHash
	if overlap != nil {
		overlap.Annotate(manifest)
	} else if _, err := analyticsStore.AnnotateImportManifestOverlap(ctx, siteID, manifest); err != nil {
		slog.Error("Failed to annotate completed import overlap", "error", err, "site_id", siteID, "import_id", importID)
	}
	if err := h.ctx.Store.MarkImportCompleted(ctx, siteID, importID, sink.Rows(), manifest); err != nil {
		slog.Error("Failed to mark import completed", "error", err, "import_id", importID)
		markFailed("could not mark import completed")
		return
	}
	if sink.Rows() > 0 {
		h.appendImportAudit(ctx, nil, siteID, importID, actorID, job.Provider, "import.data_written", "success", fmt.Sprintf("Wrote %d imported row(s)", sink.Rows()))
	}
	h.appendImportAudit(ctx, nil, siteID, importID, actorID, job.Provider, "import.completed", "success", fmt.Sprintf("%s import completed with %d imported row(s)", job.Provider, sink.Rows()))
	h.cleanupStagedFiles(importID)
}

func (h *handler) sourceSet(ctx context.Context, siteID, importID uuid.UUID, hashFiles bool) (importables.SourceSet, error) {
	files, err := h.ctx.Store.ListImportFiles(ctx, importID)
	if err != nil {
		return importables.SourceSet{}, err
	}
	if len(files) == 0 {
		return importables.SourceSet{}, fmt.Errorf("no upload files found")
	}
	sourceFiles := make([]importables.SourceFile, 0, len(files))
	for _, file := range files {
		if file.BytesReceived < file.SizeBytes {
			return importables.SourceSet{}, fmt.Errorf("file %s is not fully uploaded", file.Filename)
		}
		path := h.stagedPath(file.RelativePath)
		stat, err := os.Stat(path)
		if err != nil {
			return importables.SourceSet{}, fmt.Errorf("staged file %s is missing", file.Filename)
		}
		if stat.Size() != file.SizeBytes {
			return importables.SourceSet{}, fmt.Errorf("staged file %s size does not match upload metadata", file.Filename)
		}
		sourceFiles = append(sourceFiles, importables.SourceFile{
			ID:        file.ID,
			Name:      file.Filename,
			Path:      path,
			SizeBytes: file.SizeBytes,
			SHA256:    file.SHA256,
		})
	}
	sort.Slice(sourceFiles, func(i, j int) bool {
		if sourceFiles[i].Name == sourceFiles[j].Name {
			return sourceFiles[i].ID.String() < sourceFiles[j].ID.String()
		}
		return sourceFiles[i].Name < sourceFiles[j].Name
	})
	var sourceHash string
	if hashFiles {
		sourceHash, err = h.hashSourceSet(ctx, importID, sourceFiles)
		if err != nil {
			return importables.SourceSet{}, err
		}
	} else {
		sourceHash = combinedSourceHash(sourceFiles)
	}
	return importables.SourceSet{Files: sourceFiles, SourceHash: sourceHash, SiteDomain: h.siteDomain(ctx, siteID)}, nil
}

func (h *handler) siteDomain(ctx context.Context, siteID uuid.UUID) string {
	site, err := h.ctx.Store.GetSiteByID(ctx, siteID)
	if err != nil || site == nil {
		return ""
	}
	return strings.TrimSpace(site.Domain)
}

func (h *handler) hashSourceSet(ctx context.Context, importID uuid.UUID, files []importables.SourceFile) (string, error) {
	for i := range files {
		hash, err := hashFile(files[i].Path)
		if err != nil {
			return "", err
		}
		expected := strings.ToLower(strings.TrimSpace(files[i].SHA256))
		if expected != "" && expected != hash {
			return "", fmt.Errorf("checksum mismatch for %s", files[i].Name)
		}
		files[i].SHA256 = hash
		if err := h.ctx.Store.UpdateImportFileProgress(ctx, importID, files[i].ID, files[i].SizeBytes, hash); err != nil {
			return "", err
		}
	}
	return combinedSourceHash(files), nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open staged file for hashing: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("hash staged file: %w", err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func combinedSourceHash(files []importables.SourceFile) string {
	files = append([]importables.SourceFile(nil), files...)
	sort.Slice(files, func(i, j int) bool {
		if files[i].SHA256 == files[j].SHA256 {
			return files[i].SizeBytes < files[j].SizeBytes
		}
		return files[i].SHA256 < files[j].SHA256
	})
	hash := sha256.New()
	for _, file := range files {
		_, _ = hash.Write([]byte(strconv.FormatInt(file.SizeBytes, 10)))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(file.SHA256))
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (h *handler) stagedPath(relativePath string) string {
	clean := filepath.Clean(relativePath)
	return filepath.Join(h.ctx.Config.DataPath, clean)
}

func (h *handler) cleanupStagedFiles(importID uuid.UUID) {
	files, err := h.ctx.Store.ListImportFiles(context.Background(), importID)
	if err != nil {
		return
	}
	cleanedAt := time.Now().UTC()
	for _, file := range files {
		if err := os.Remove(h.stagedPath(file.RelativePath)); err == nil || os.IsNotExist(err) {
			_ = h.ctx.Store.MarkImportFileCleaned(context.Background(), importID, file.ID, cleanedAt)
		}
	}
}

var (
	unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	sha256HexPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func sanitizeUploadFilename(filename string) string {
	base := filepath.Base(strings.TrimSpace(filename))
	base = unsafeFilenameChars.ReplaceAllString(base, "_")
	base = strings.Trim(base, "._-")
	if base == "" {
		return ""
	}
	return base
}

func parseUUIDPath(w http.ResponseWriter, r *http.Request, name, message string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue(name)))
	if err != nil {
		http.Error(w, message, http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}

func contextWithoutCancel() context.Context {
	return context.Background()
}
