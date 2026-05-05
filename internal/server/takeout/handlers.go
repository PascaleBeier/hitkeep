package takeout

import (
	"net/http"

	"github.com/google/uuid"

	authcore "hitkeep/internal/auth"
	"hitkeep/internal/exportfmt"
	"hitkeep/internal/server/shared"
	takeoutsvc "hitkeep/internal/takeout"
)

type TakeoutHandler struct {
	service *takeoutsvc.TakeoutService
}

func NewTakeoutHandler(service *takeoutsvc.TakeoutService) *TakeoutHandler {
	return &TakeoutHandler{
		service: service,
	}
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	handler := NewTakeoutHandler(ctx.Takeout)
	mux.HandleFunc("GET /api/user/takeout", ctx.Handler(shared.HandlerConfig{
		RequireAuth: true,
		RateLimiter: ctx.ApiLimiter,
	}, handler.handleUserTakeout()))
	mux.HandleFunc("GET /api/sites/{id}/takeout", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, handler.handleSiteTakeout()))
}

func (h *TakeoutHandler) handleUserTakeout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := shared.GetUserIDFromContext(r)
		if userID == uuid.Nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		format := exportfmt.Normalize(r.URL.Query().Get("format"), exportfmt.FormatXLSX)

		filename, err := h.service.ExportUserData(r.Context(), userID, format)
		if err != nil {
			http.Error(w, "Failed to export user data", http.StatusInternalServerError)
			return
		}

		h.serveTakeoutFile(w, r, filename)

		go func() {
			h.service.CleanupExportFile(filename)
		}()
	}
}
func (h *TakeoutHandler) handleSiteTakeout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteIDStr := r.PathValue("id")
		siteID, err := uuid.Parse(siteIDStr)
		if err != nil {
			http.Error(w, "Invalid site ID", http.StatusBadRequest)
			return
		}

		// Validate format
		// Default to xlsx for better compatibility with mixed schema exports (hits + events)
		format := exportfmt.Normalize(r.URL.Query().Get("format"), exportfmt.FormatXLSX)

		filename, err := h.service.ExportSiteData(r.Context(), siteID, format)
		if err != nil {
			http.Error(w, "Failed to export site data", http.StatusInternalServerError)
			return
		}

		h.serveTakeoutFile(w, r, filename)

		go func() {
			h.service.CleanupExportFile(filename)
		}()
	}
}

func (h *TakeoutHandler) serveTakeoutFile(w http.ResponseWriter, r *http.Request, filename string) {
	exportFile, err := h.service.OpenExportFile(filename)
	if err != nil {
		http.Error(w, "Failed to export file", http.StatusInternalServerError)
		return
	}
	defer func() {
		_ = exportFile.File.Close()
	}()

	downloadName := exportFile.Name
	w.Header().Set("Content-Disposition", "attachment; filename="+downloadName)
	w.Header().Set("Content-Type", exportfmt.ContentTypeForFilename(downloadName))
	http.ServeContent(w, r, downloadName, exportFile.Info.ModTime(), exportFile.File)
}
