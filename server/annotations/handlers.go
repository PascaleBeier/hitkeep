// Package annotations serves team-visible notes on a site's timeline.
package annotations

import (
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"hitkeep/api"
	authcore "hitkeep/auth"
	"hitkeep/database"
	json "hitkeep/jsonapi"
	"hitkeep/server/filterparams"
	"hitkeep/server/shared"
)

const maxBodyRunes = 280

// maxFutureOffset bounds how far ahead a note may be placed: far enough for
// a planned launch, close enough to catch a mistyped year.
const maxFutureOffset = 366 * 24 * time.Hour

type handler struct {
	ctx *shared.Context
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}
	mux.HandleFunc("GET /api/sites/{id}/annotations", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleList()))
	mux.HandleFunc("POST /api/sites/{id}/annotations", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageAnnotations,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreate()))
	mux.HandleFunc("PUT /api/sites/{id}/annotations/{annotationID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageAnnotations,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdate()))
	mux.HandleFunc("DELETE /api/sites/{id}/annotations/{annotationID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageAnnotations,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDelete()))
}

func (h *handler) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := shared.PathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		start, end := filterparams.ParseLenientAnalyticsRange(r.URL.Query())
		annotations, err := h.ctx.Store.ListAnnotations(r.Context(), siteID, start, end)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list annotations", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(r.Context(), w, http.StatusOK, annotations)
	}
}

func (h *handler) handleCreate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := shared.PathUUID(w, r, "id", "Invalid site_id")
		if !ok {
			return
		}
		input, ok := decodeInput(w, r)
		if !ok {
			return
		}
		created, err := h.ctx.Store.CreateAnnotation(r.Context(), siteID, input, shared.GetUserIDFromContext(r))
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create annotation", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(r.Context(), w, http.StatusCreated, created)
	}
}

func (h *handler) handleUpdate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, annotationID, ok := pathIDs(w, r)
		if !ok {
			return
		}
		input, ok := decodeInput(w, r)
		if !ok {
			return
		}
		updated, err := h.ctx.Store.UpdateAnnotation(r.Context(), siteID, annotationID, input)
		if errors.Is(err, database.ErrAnnotationNotFound) {
			http.Error(w, "Annotation not found", http.StatusNotFound)
			return
		}
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to update annotation", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		shared.WriteJSON(r.Context(), w, http.StatusOK, updated)
	}
}

func (h *handler) handleDelete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, annotationID, ok := pathIDs(w, r)
		if !ok {
			return
		}
		err := h.ctx.Store.DeleteAnnotation(r.Context(), siteID, annotationID)
		if errors.Is(err, database.ErrAnnotationNotFound) {
			http.Error(w, "Annotation not found", http.StatusNotFound)
			return
		}
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to delete annotation", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func pathIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	siteID, ok := shared.PathUUID(w, r, "id", "Invalid site_id")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	annotationID, ok := shared.PathUUID(w, r, "annotationID", "Invalid annotation id")
	return siteID, annotationID, ok
}

// decodeInput reads and validates a note. The site always comes from the
// path, so the body carries no owner fields at all.
func decodeInput(w http.ResponseWriter, r *http.Request) (api.AnnotationInput, bool) {
	var input api.AnnotationInput
	if err := json.UnmarshalRead(r.Body, &input); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return input, false
	}
	if msg := validateInput(&input, time.Now()); msg != "" {
		http.Error(w, msg, http.StatusBadRequest)
		return input, false
	}
	return input, true
}

func validateInput(input *api.AnnotationInput, now time.Time) string {
	input.Body = strings.TrimSpace(input.Body)
	switch {
	case input.Body == "":
		return "Annotation text is required"
	case utf8.RuneCountInString(input.Body) > maxBodyRunes:
		return "Annotation text is too long"
	case input.StartsAt.IsZero():
		return "starts_at is required"
	case input.EndsAt != nil && input.EndsAt.Before(input.StartsAt):
		return "ends_at must not be before starts_at"
	case input.StartsAt.After(now.Add(maxFutureOffset)):
		return "starts_at is too far in the future"
	}
	return ""
}
