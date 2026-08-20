package qrcodes

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/netip"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/appurl"
	"hitkeep/internal/assetstore"
	authcore "hitkeep/internal/auth"
	"hitkeep/internal/blocking"
	"hitkeep/internal/exportfmt"
	"hitkeep/internal/ipmeta"
	json "hitkeep/internal/jsonapi"
	"hitkeep/internal/server/shared"
)

const maxAssetBytes = 2 << 20

type handler struct {
	ctx              *shared.Context
	lookupIPMetadata func(netip.Addr) ipmeta.Metadata
}

func Register(mux *http.ServeMux, ctx *shared.Context) {
	h := &handler{ctx: ctx}

	mux.HandleFunc("GET /q/{token}", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.IngestLimiter,
	}, h.handleRedirect()))

	mux.HandleFunc("GET /api/sites/{id}/qr-codes", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleList()))
	mux.HandleFunc("POST /api/sites/{id}/qr-codes", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreate()))
	mux.HandleFunc("GET /api/sites/{id}/qr-codes/{qrID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGet()))
	mux.HandleFunc("PATCH /api/sites/{id}/qr-codes/{qrID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleUpdate()))
	mux.HandleFunc("DELETE /api/sites/{id}/qr-codes/{qrID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleArchive()))
	mux.HandleFunc("PUT /api/sites/{id}/qr-codes/{qrID}/asset", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handlePutAsset()))
	mux.HandleFunc("GET /api/sites/{id}/qr-codes/{qrID}/asset", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleGetAsset()))
	mux.HandleFunc("DELETE /api/sites/{id}/qr-codes/{qrID}/asset", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteAsset()))
	mux.HandleFunc("GET /api/sites/{id}/qr-codes/{qrID}/summary", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleSummary()))
	mux.HandleFunc("GET /api/sites/{id}/qr-codes/{qrID}/opens/timeseries", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleOpenSeries()))
	mux.HandleFunc("GET /api/sites/{id}/qr-codes/{qrID}/takeout", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleTakeout()))
	mux.HandleFunc("GET /api/sites/{id}/qr-codes/{qrID}/share", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteView,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleListShares()))
	mux.HandleFunc("POST /api/sites/{id}/qr-codes/{qrID}/share", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleCreateShare()))
	mux.HandleFunc("DELETE /api/sites/{id}/qr-codes/{qrID}/share/{shareID}", ctx.Handler(shared.HandlerConfig{
		SitePerm:    authcore.PermSiteManageData,
		RateLimiter: ctx.ApiLimiter,
	}, h.handleDeleteShare()))

	mux.HandleFunc("GET /api/share/{token}/sites/{id}/qr-codes", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleShareList()))
	mux.HandleFunc("GET /api/share/{token}/sites/{id}/qr-codes/{qrID}", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleShareGet()))
	mux.HandleFunc("GET /api/share/{token}/sites/{id}/qr-codes/{qrID}/asset", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleShareAsset()))
	mux.HandleFunc("GET /api/share/{token}/sites/{id}/qr-codes/{qrID}/summary", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleShareSummary()))
	mux.HandleFunc("GET /api/share/{token}/sites/{id}/qr-codes/{qrID}/opens/timeseries", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleShareOpenSeries()))
	mux.HandleFunc("GET /api/share/{token}/sites/{id}/qr-codes/{qrID}/takeout", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleShareTakeout()))

	mux.HandleFunc("GET /api/qr-share/{token}/qr-code", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleQRShareGet()))
	mux.HandleFunc("GET /api/qr-share/{token}/qr-code/asset", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleQRShareAsset()))
	mux.HandleFunc("GET /api/qr-share/{token}/qr-code/summary", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleQRShareSummary()))
	mux.HandleFunc("GET /api/qr-share/{token}/qr-code/opens/timeseries", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleQRShareOpenSeries()))
	mux.HandleFunc("GET /api/qr-share/{token}/qr-code/takeout", ctx.Handler(shared.HandlerConfig{
		RateLimiter: ctx.ApiLimiter,
	}, h.handleQRShareTakeout()))
}

func (h *handler) handleList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok || !h.ensureStore(w) {
			return
		}
		includeArchived := strings.EqualFold(r.URL.Query().Get("include_archived"), "true")
		qrs, err := h.ctx.Store.ListQRCodes(r.Context(), siteID, includeArchived)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list QR codes", "error", err, "site_id", siteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.withRedirectURLs(qrs)
		writeJSON(r.Context(), w, qrs)
	}
}

func (h *handler) handleCreate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
		if !ok || !h.ensureStore(w) {
			return
		}
		req, ok := decodeQRRequest(w, r)
		if !ok {
			return
		}
		userID := shared.GetUserIDFromContext(r)
		qr, token, err := h.ctx.Store.CreateQRCode(r.Context(), siteID, userID, req)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create QR code", "error", err, "site_id", siteID, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		qr.RedirectURL = h.redirectURL(token)
		w.WriteHeader(http.StatusCreated)
		writeJSON(r.Context(), w, qr)
	}
}

func (h *handler) handleGet() http.HandlerFunc {
	return h.qrHandler(func(w http.ResponseWriter, r *http.Request, qr *api.QRCode) {
		h.withRedirectURL(qr)
		writeJSON(r.Context(), w, qr)
	})
}

func (h *handler) handleUpdate() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, qrID, ok := parseSiteQR(w, r)
		if !ok || !h.ensureStore(w) {
			return
		}
		req, ok := decodeQRRequest(w, r)
		if !ok {
			return
		}
		qr, err := h.ctx.Store.UpdateQRCode(r.Context(), siteID, qrID, req)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to update QR code", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if qr == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		h.withRedirectURL(qr)
		writeJSON(r.Context(), w, qr)
	}
}

func (h *handler) handleArchive() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, qrID, ok := parseSiteQR(w, r)
		if !ok || !h.ensureStore(w) {
			return
		}
		archived, err := h.ctx.Store.ArchiveQRCode(r.Context(), siteID, qrID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to archive QR code", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !archived {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handlePutAsset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, qrID, ok := parseSiteQR(w, r)
		if !ok || !h.ensureStore(w) {
			return
		}
		if !h.ensureQRExists(w, r, siteID, qrID) {
			return
		}
		asset, ok := readAsset(w, r, siteID, qrID)
		if !ok {
			return
		}
		previous, err := h.ctx.Store.GetQRCodeAsset(r.Context(), siteID, qrID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to read previous QR asset", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		storageKey, err := h.assetStore().PutQRCodeAsset(siteID, qrID, asset.Checksum, asset.Filename, asset.ContentType, asset.Data)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to store QR asset file", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		asset.StorageKey = storageKey
		asset.Data = nil
		saved, err := h.ctx.Store.UpsertQRCodeAsset(r.Context(), asset)
		if err != nil {
			if deleteErr := h.assetStore().Delete(storageKey); deleteErr != nil {
				shared.LoggerFromContext(r.Context()).Warn("Failed to delete QR asset file after metadata save failure", "error", deleteErr, "site_id", siteID, "qr_code_id", qrID, "storage_key", storageKey)
			}
			shared.LoggerFromContext(r.Context()).Error("Failed to save QR asset", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if previous != nil && previous.StorageKey != "" && previous.StorageKey != storageKey {
			if err := h.assetStore().Delete(previous.StorageKey); err != nil {
				shared.LoggerFromContext(r.Context()).Warn("Failed to delete replaced QR asset file", "error", err, "site_id", siteID, "qr_code_id", qrID, "storage_key", previous.StorageKey)
			}
		}
		writeJSON(r.Context(), w, saved)
	}
}

func (h *handler) handleGetAsset() http.HandlerFunc {
	return h.assetHandler(func(w http.ResponseWriter, r *http.Request, asset *api.QRCodeAsset) {
		h.serveAsset(w, r, asset)
	})
}

func (h *handler) handleDeleteAsset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, qrID, ok := parseSiteQR(w, r)
		if !ok || !h.ensureStore(w) {
			return
		}
		asset, err := h.ctx.Store.GetQRCodeAsset(r.Context(), siteID, qrID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get QR asset before delete", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if asset == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		deleted, err := h.ctx.Store.DeleteQRCodeAsset(r.Context(), siteID, qrID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to delete QR asset", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !deleted {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		if asset.StorageKey != "" {
			if err := h.assetStore().Delete(asset.StorageKey); err != nil {
				shared.LoggerFromContext(r.Context()).Warn("Failed to delete QR asset file", "error", err, "site_id", siteID, "qr_code_id", qrID, "storage_key", asset.StorageKey)
			}
		}
		if err := h.assetStore().DeleteQRCodeAssetDir(siteID, qrID); err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Failed to delete QR asset directory", "error", err, "site_id", siteID, "qr_code_id", qrID)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handleSummary() http.HandlerFunc {
	return h.summaryHandler(h.loadAuthenticatedQR)
}

func (h *handler) handleOpenSeries() http.HandlerFunc {
	return h.openSeriesHandler(h.loadAuthenticatedQR)
}

func (h *handler) handleTakeout() http.HandlerFunc {
	return h.takeoutHandler(h.loadAuthenticatedQR)
}

func (h *handler) handleListShares() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, qrID, ok := parseSiteQR(w, r)
		if !ok || !h.ensureStore(w) || !h.ensureQRExists(w, r, siteID, qrID) {
			return
		}
		links, err := h.ctx.Store.ListQRCodeShareLinks(r.Context(), siteID, qrID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list QR shares", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(r.Context(), w, links)
	}
}

func (h *handler) handleCreateShare() http.HandlerFunc {
	type response struct {
		api.QRCodeShareLink
		Token string `json:"token"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, qrID, ok := parseSiteQR(w, r)
		if !ok || !h.ensureStore(w) || !h.ensureQRExists(w, r, siteID, qrID) {
			return
		}
		link, token, err := h.ctx.Store.CreateQRCodeShareLink(r.Context(), siteID, qrID, shared.GetUserIDFromContext(r))
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to create QR share", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		link.URL = appurl.Path(h.ctx.Config.PublicURL, "/qr-share/"+token)
		w.WriteHeader(http.StatusCreated)
		writeJSON(r.Context(), w, response{QRCodeShareLink: *link, Token: token})
	}
}

func (h *handler) handleDeleteShare() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, qrID, ok := parseSiteQR(w, r)
		if !ok || !h.ensureStore(w) {
			return
		}
		shareID, ok := parseUUIDPath(w, r, "shareID", "Invalid share_id")
		if !ok {
			return
		}
		revoked, err := h.ctx.Store.RevokeQRCodeShareLink(r.Context(), siteID, qrID, shareID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to delete QR share", "error", err, "site_id", siteID, "qr_code_id", qrID, "share_id", shareID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !revoked {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *handler) handleRedirect() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.ensureStore(w) {
			return
		}
		token := strings.TrimSpace(r.PathValue("token"))
		if token == "" {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return
		}
		qr, err := h.ctx.Store.GetQRCodeByToken(r.Context(), token)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve QR redirect", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if qr == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		destination, err := buildDestinationURL(*qr)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to build QR redirect destination", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
			http.Error(w, "Invalid destination", http.StatusInternalServerError)
			return
		}
		h.recordOpenBestEffort(r.Context(), r, qr)
		//nolint:gosec // QR campaigns intentionally redirect to saved absolute http(s) destinations validated by buildDestinationURL.
		http.Redirect(w, r, destination, http.StatusFound)
	}
}

func (h *handler) handleShareList() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site, ok := h.loadSiteShare(w, r)
		if !ok {
			return
		}
		qrs, err := h.ctx.Store.ListQRCodes(r.Context(), site.ID, false)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to list shared QR codes", "error", err, "site_id", site.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		h.withRedirectURLs(qrs)
		writeJSON(r.Context(), w, qrs)
	}
}

func (h *handler) handleShareGet() http.HandlerFunc {
	return h.shareQRHandler(func(w http.ResponseWriter, r *http.Request, qr *api.QRCode) {
		h.withRedirectURL(qr)
		writeJSON(r.Context(), w, qr)
	})
}

func (h *handler) handleShareAsset() http.HandlerFunc {
	return h.shareAssetHandler(func(w http.ResponseWriter, r *http.Request, asset *api.QRCodeAsset) {
		h.serveAsset(w, r, asset)
	})
}

func (h *handler) handleShareSummary() http.HandlerFunc {
	return h.summaryHandler(h.loadSiteSharedQR)
}

func (h *handler) handleShareOpenSeries() http.HandlerFunc {
	return h.openSeriesHandler(h.loadSiteSharedQR)
}

func (h *handler) handleShareTakeout() http.HandlerFunc {
	return h.takeoutHandler(h.loadSiteSharedQR)
}

func (h *handler) handleQRShareGet() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qr, ok := h.loadQRShare(w, r)
		if !ok {
			return
		}
		h.withRedirectURL(qr)
		writeJSON(r.Context(), w, qr)
	}
}

func (h *handler) handleQRShareAsset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qr, ok := h.loadQRShare(w, r)
		if !ok {
			return
		}
		asset, err := h.ctx.Store.GetQRCodeAsset(r.Context(), qr.SiteID, qr.ID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get QR share asset", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if asset == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		h.serveAsset(w, r, asset)
	}
}

func (h *handler) handleQRShareSummary() http.HandlerFunc {
	return h.summaryHandler(h.loadQRSharedQR)
}

func (h *handler) handleQRShareOpenSeries() http.HandlerFunc {
	return h.openSeriesHandler(h.loadQRSharedQR)
}

func (h *handler) handleQRShareTakeout() http.HandlerFunc {
	return h.takeoutHandler(h.loadQRSharedQR)
}

type loadQRFunc func(http.ResponseWriter, *http.Request) (*api.QRCode, bool)

func (h *handler) summaryHandler(load loadQRFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qr, ok := load(w, r)
		if !ok {
			return
		}
		start, end := parseRange(r)
		stats, opens, ok := h.loadQRStats(w, r, qr, start, end)
		if !ok {
			return
		}
		h.withRedirectURL(qr)
		writeJSON(r.Context(), w, api.QRCodeSummary{
			QRCode:       *qr,
			OpenCount:    opens,
			Pageviews:    stats.TotalPageviews,
			Visitors:     stats.UniqueSessions,
			TopPages:     stats.TopPages,
			TopReferrers: stats.TopReferrers,
			TopDevices:   stats.TopDevices,
			TopCountries: stats.TopCountries,
		})
	}
}

func (h *handler) openSeriesHandler(load loadQRFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qr, ok := load(w, r)
		if !ok {
			return
		}
		start, end := parseRange(r)
		store, err := h.ctx.AnalyticsStore(r.Context(), qr.SiteID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to resolve QR analytics store", "error", err, "site_id", qr.SiteID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		points, err := store.GetQRCodeOpenSeries(r.Context(), qr.SiteID, qr.ID, start, end)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get QR open series", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		writeJSON(r.Context(), w, points)
	}
}

func (h *handler) takeoutHandler(load loadQRFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qr, ok := load(w, r)
		if !ok {
			return
		}
		if h.ctx.Takeout == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}
		format := exportfmt.Normalize(r.URL.Query().Get("format"), exportfmt.FormatXLSX)
		filename, err := h.ctx.Takeout.ExportQRCodeData(r.Context(), qr.SiteID, qr.ID, format)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to export QR takeout", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
			http.Error(w, "Failed to export QR data", http.StatusInternalServerError)
			return
		}
		defer h.ctx.Takeout.CleanupExportFile(filename)
		exportFile, err := h.ctx.Takeout.OpenExportFile(filename)
		if err != nil {
			http.Error(w, "Failed to export file", http.StatusInternalServerError)
			return
		}
		defer exportFile.File.Close()
		w.Header().Set("Content-Type", exportfmt.ContentTypeForFilename(exportFile.Name))
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, exportFile.Name))
		http.ServeContent(w, r, exportFile.Name, exportFile.Info.ModTime(), exportFile.File)
	}
}

func (h *handler) loadQRStats(w http.ResponseWriter, r *http.Request, qr *api.QRCode, start, end time.Time) (*api.SiteStats, int, bool) {
	store, err := h.ctx.AnalyticsStore(r.Context(), qr.SiteID)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to resolve QR analytics store", "error", err, "site_id", qr.SiteID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, 0, false
	}
	filters := []api.Filter{{Type: "qr_code_id", Value: qr.ID.String()}}
	stats, err := store.GetSiteStats(r.Context(), api.AnalyticsParams{
		SiteID:  qr.SiteID,
		UserID:  qr.CreatedBy,
		Start:   start,
		End:     end,
		Filters: filters,
	})
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to get QR stats", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, 0, false
	}
	opens, err := store.CountQRCodeOpens(r.Context(), qr.SiteID, qr.ID, start, end)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to count QR opens", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, 0, false
	}
	return stats, opens, true
}

func (h *handler) qrHandler(fn func(http.ResponseWriter, *http.Request, *api.QRCode)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qr, ok := h.loadAuthenticatedQR(w, r)
		if !ok {
			return
		}
		fn(w, r, qr)
	}
}

func (h *handler) assetHandler(fn func(http.ResponseWriter, *http.Request, *api.QRCodeAsset)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		siteID, qrID, ok := parseSiteQR(w, r)
		if !ok || !h.ensureStore(w) {
			return
		}
		asset, err := h.ctx.Store.GetQRCodeAsset(r.Context(), siteID, qrID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get QR asset", "error", err, "site_id", siteID, "qr_code_id", qrID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if asset == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		fn(w, r, asset)
	}
}

func (h *handler) shareQRHandler(fn func(http.ResponseWriter, *http.Request, *api.QRCode)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qr, ok := h.loadSiteSharedQR(w, r)
		if !ok {
			return
		}
		fn(w, r, qr)
	}
}

func (h *handler) shareAssetHandler(fn func(http.ResponseWriter, *http.Request, *api.QRCodeAsset)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		qr, ok := h.loadSiteSharedQR(w, r)
		if !ok {
			return
		}
		asset, err := h.ctx.Store.GetQRCodeAsset(r.Context(), qr.SiteID, qr.ID)
		if err != nil {
			shared.LoggerFromContext(r.Context()).Error("Failed to get shared QR asset", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if asset == nil {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		fn(w, r, asset)
	}
}

func (h *handler) loadAuthenticatedQR(w http.ResponseWriter, r *http.Request) (*api.QRCode, bool) {
	siteID, qrID, ok := parseSiteQR(w, r)
	if !ok || !h.ensureStore(w) {
		return nil, false
	}
	qr, err := h.ctx.Store.GetQRCode(r.Context(), siteID, qrID)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to get QR code", "error", err, "site_id", siteID, "qr_code_id", qrID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if qr == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, false
	}
	return qr, true
}

func (h *handler) loadSiteSharedQR(w http.ResponseWriter, r *http.Request) (*api.QRCode, bool) {
	site, ok := h.loadSiteShare(w, r)
	if !ok {
		return nil, false
	}
	qrID, ok := parseUUIDPath(w, r, "qrID", "Invalid qr_code_id")
	if !ok {
		return nil, false
	}
	qr, err := h.ctx.Store.GetQRCode(r.Context(), site.ID, qrID)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to get site-shared QR code", "error", err, "site_id", site.ID, "qr_code_id", qrID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if qr == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, false
	}
	return qr, true
}

func (h *handler) loadQRSharedQR(w http.ResponseWriter, r *http.Request) (*api.QRCode, bool) {
	return h.loadQRShare(w, r)
}

func (h *handler) loadQRShare(w http.ResponseWriter, r *http.Request) (*api.QRCode, bool) {
	if !h.ensureStore(w) {
		return nil, false
	}
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return nil, false
	}
	qr, err := h.ctx.Store.GetQRCodeByShareToken(r.Context(), token)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to get QR share", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if qr == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, false
	}
	return qr, true
}

func (h *handler) loadSiteShare(w http.ResponseWriter, r *http.Request) (*api.Site, bool) {
	if !h.ensureStore(w) {
		return nil, false
	}
	token := strings.TrimSpace(r.PathValue("token"))
	if token == "" {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return nil, false
	}
	site, err := h.ctx.Store.GetShareSiteByToken(r.Context(), token)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to load QR site share", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return nil, false
	}
	if site == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, false
	}
	pathSiteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
	if !ok {
		return nil, false
	}
	if pathSiteID != site.ID {
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, false
	}
	return site, true
}

func (h *handler) ensureQRExists(w http.ResponseWriter, r *http.Request, siteID, qrID uuid.UUID) bool {
	qr, err := h.ctx.Store.GetQRCode(r.Context(), siteID, qrID)
	if err != nil {
		shared.LoggerFromContext(r.Context()).Error("Failed to verify QR code", "error", err, "site_id", siteID, "qr_code_id", qrID)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return false
	}
	if qr == nil {
		http.Error(w, "Not found", http.StatusNotFound)
		return false
	}
	return true
}

func (h *handler) recordOpenBestEffort(ctx context.Context, r *http.Request, qr *api.QRCode) {
	if respectsDNT(r) {
		return
	}
	store, err := h.ctx.AnalyticsStore(ctx, qr.SiteID)
	if err != nil {
		shared.LoggerFromContext(ctx).Warn("Skipping QR open because analytics store could not be resolved", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
		return
	}
	userIP, countryCode, metadata := h.qrOpenRequestContext(r)
	referrer := stringPtrIfNotEmpty(r.Referer())
	if h.shouldSuppressQROpen(qr, userIP, countryCode, r.UserAgent(), referrer) {
		return
	}
	if err := store.CreateQRCodeOpen(ctx, &api.QRCodeOpen{
		SiteID:      qr.SiteID,
		QRCodeID:    qr.ID,
		Timestamp:   time.Now().UTC(),
		Referrer:    referrer,
		UserAgent:   stringPtrIfNotEmpty(r.UserAgent()),
		CountryCode: stringPtrIfNotEmpty(countryCode),
		Region:      stringPtrIfNotEmpty(metadata.Region),
		City:        stringPtrIfNotEmpty(metadata.City),
		Provider:    stringPtrIfNotEmpty(metadata.Provider),
		ASN:         intPtrIfPositive(metadata.ASN),
		ASNOrg:      stringPtrIfNotEmpty(metadata.ASNOrg),
	}); err != nil {
		shared.LoggerFromContext(ctx).Warn("Failed to record QR open", "error", err, "site_id", qr.SiteID, "qr_code_id", qr.ID)
	}
}

func (h *handler) qrOpenRequestContext(r *http.Request) (string, string, ipmeta.Metadata) {
	trusted := []netip.Prefix(nil)
	if h.ctx.Config != nil {
		trusted = h.ctx.Config.GetTrustedProxyNetworks()
	}
	userIP := shared.GetRealIP(r, trusted)
	metadata := ipmeta.Metadata{}
	if parsedIP, ok := shared.ParseAddr(userIP); ok {
		lookup := h.lookupIPMetadata
		if lookup == nil {
			lookup = ipmeta.Lookup
		}
		metadata = lookup(parsedIP)
	}
	countryCode := shared.CountryCodeFromRequestWithResolver(r, trusted, func(netip.Addr) string {
		return metadata.CountryCode
	})
	return userIP, countryCode, metadata
}

func (h *handler) shouldSuppressQROpen(qr *api.QRCode, userIP, countryCode, userAgent string, referrer *string) bool {
	if h.ctx.IPFilter != nil && h.ctx.IPFilter.EvaluateTraffic(qr.SiteID, blocking.TrafficExclusionContext{
		IP:          userIP,
		CountryCode: countryCode,
		UserAgent:   userAgent,
		Path:        qr.DestinationURL,
	}).Blocked {
		return true
	}
	if h.ctx.SpamFilter == nil {
		return false
	}
	return h.ctx.SpamFilter.Evaluate(qr.Name, userIP, referrer).Blocked
}

func (h *handler) withRedirectURLs(qrs []api.QRCode) {
	for i := range qrs {
		h.withRedirectURL(&qrs[i])
	}
}

func (h *handler) withRedirectURL(qr *api.QRCode) {
	if qr == nil || qr.TokenHint == "" {
		return
	}
	token := qr.RedirectToken
	if token == "" {
		token = qr.TokenHint
	}
	qr.RedirectURL = h.redirectURL(token)
}

func (h *handler) redirectURL(token string) string {
	return appurl.Path(h.ctx.Config.PublicURL, "/q/"+token)
}

func (h *handler) assetStore() *assetstore.Store {
	if h.ctx != nil && h.ctx.Config != nil {
		return assetstore.New(h.ctx.Config.DataPath)
	}
	return assetstore.New("")
}

func buildDestinationURL(qr api.QRCode) (string, error) {
	parsed, err := parseDestinationURL(qr.DestinationURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	applyCustomQueryParams(query, qr.CustomParams)
	setQueryValue(query, "utm_source", qr.UTMSource)
	setQueryValue(query, "utm_medium", qr.UTMMedium)
	setQueryValue(query, "utm_campaign", qr.UTMCampaign)
	setQueryValue(query, "utm_term", qr.UTMTerm)
	setQueryValue(query, "utm_content", qr.UTMContent)
	query.Set("hk_qr", qr.ID.String())
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func parseDestinationURL(rawURL string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid destination url")
	}
	return parsed, nil
}

func applyCustomQueryParams(query url.Values, params map[string]string) {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(params[key])
		if key != "" && value != "" && !strings.EqualFold(key, "hk_qr") {
			query.Set(key, value)
		}
	}
}

func decodeQRRequest(w http.ResponseWriter, r *http.Request) (api.QRCodeCreateRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	var req api.QRCodeCreateRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return req, false
	}
	req = normalizeQRRequest(req)
	if !validateQRRequest(w, req) {
		return req, false
	}
	req.CustomParams = normalizeCustomParams(req.CustomParams)
	if len(req.CustomParams) > 20 {
		http.Error(w, "too many custom params", http.StatusBadRequest)
		return req, false
	}
	return req, true
}

func normalizeQRRequest(req api.QRCodeCreateRequest) api.QRCodeCreateRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.DestinationURL = strings.TrimSpace(req.DestinationURL)
	req.UTMSource = strings.TrimSpace(req.UTMSource)
	req.UTMMedium = strings.TrimSpace(req.UTMMedium)
	req.UTMCampaign = strings.TrimSpace(req.UTMCampaign)
	req.UTMTerm = strings.TrimSpace(req.UTMTerm)
	req.UTMContent = strings.TrimSpace(req.UTMContent)
	return req
}

func validateQRRequest(w http.ResponseWriter, req api.QRCodeCreateRequest) bool {
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return false
	}
	if len(req.Name) > 120 {
		http.Error(w, "name is too long", http.StatusBadRequest)
		return false
	}
	if _, err := url.ParseRequestURI(req.DestinationURL); err != nil {
		http.Error(w, "destination_url must be valid", http.StatusBadRequest)
		return false
	}
	if _, err := parseDestinationURL(req.DestinationURL); err != nil {
		http.Error(w, "destination_url must be an absolute http or https URL", http.StatusBadRequest)
		return false
	}
	return true
}

func readAsset(w http.ResponseWriter, r *http.Request, siteID, qrID uuid.UUID) (api.QRCodeAsset, bool) {
	body, header, ok := readAssetBody(w, r)
	if !ok {
		return api.QRCodeAsset{}, false
	}
	contentType, ok := assetContentType(w, header, body)
	if !ok {
		return api.QRCodeAsset{}, false
	}
	width, height := assetDimensions(contentType, body)
	sum := sha256.Sum256(body)
	return api.QRCodeAsset{
		QRCodeID:    qrID,
		SiteID:      siteID,
		Filename:    assetFilename(header.Filename),
		ContentType: contentType,
		ByteSize:    int64(len(body)),
		Width:       width,
		Height:      height,
		Checksum:    hex.EncodeToString(sum[:]),
		Data:        body,
	}, true
}

func readAssetBody(w http.ResponseWriter, r *http.Request) ([]byte, *multipart.FileHeader, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxAssetBytes+1024)
	file, header, err := r.FormFile("asset")
	if err != nil {
		http.Error(w, "asset file is required", http.StatusBadRequest)
		return nil, nil, false
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maxAssetBytes+1))
	if err != nil {
		http.Error(w, "Failed to read asset", http.StatusBadRequest)
		return nil, nil, false
	}
	if len(body) == 0 {
		http.Error(w, "asset file is empty", http.StatusBadRequest)
		return nil, nil, false
	}
	if len(body) > maxAssetBytes {
		http.Error(w, "asset file is too large", http.StatusRequestEntityTooLarge)
		return nil, nil, false
	}
	return body, header, true
}

func assetContentType(w http.ResponseWriter, header *multipart.FileHeader, body []byte) (string, bool) {
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(body)
	}
	contentType, _, _ = mime.ParseMediaType(contentType)
	switch contentType {
	case "image/png", "image/jpeg", "image/webp":
		return contentType, true
	default:
		http.Error(w, "asset must be PNG, JPEG, or WebP", http.StatusBadRequest)
		return "", false
	}
}

func assetDimensions(contentType string, body []byte) (int, int) {
	if contentType == "image/webp" {
		return 0, 0
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func assetFilename(filename string) string {
	filename = filepath.Base(filename)
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		return "qr-asset"
	}
	return filename
}

func (h *handler) serveAsset(w http.ResponseWriter, r *http.Request, asset *api.QRCodeAsset) {
	w.Header().Set("Content-Type", asset.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=300")
	if asset.StorageKey != "" {
		file, err := h.assetStore().Open(asset.StorageKey)
		if err == nil {
			defer file.Close()
			info, statErr := file.Stat()
			if statErr == nil {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
				http.ServeContent(w, r, asset.Filename, info.ModTime(), file)
				return
			}
		}
		if err != nil {
			shared.LoggerFromContext(r.Context()).Warn("Failed to open QR asset file", "error", err, "site_id", asset.SiteID, "qr_code_id", asset.QRCodeID, "storage_key", asset.StorageKey)
		}
	}
	if len(asset.Data) == 0 {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(asset.Data)))
	http.ServeContent(w, r, asset.Filename, asset.UpdatedAt, bytes.NewReader(asset.Data))
}

func parseSiteQR(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	siteID, ok := parseUUIDPath(w, r, "id", "Invalid site_id")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	qrID, ok := parseUUIDPath(w, r, "qrID", "Invalid qr_code_id")
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}
	return siteID, qrID, true
}

func parseUUIDPath(w http.ResponseWriter, r *http.Request, key, message string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(r.PathValue(key)))
	if err != nil {
		http.Error(w, message, http.StatusBadRequest)
		return uuid.Nil, false
	}
	return id, true
}

func parseRange(r *http.Request) (time.Time, time.Time) {
	now := time.Now().UTC()
	end := now.AddDate(0, 0, 1)
	start := end.AddDate(0, 0, -30)
	q := r.URL.Query()
	if raw := q.Get("from"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			start = parsed
		}
	}
	if raw := q.Get("to"); raw != "" {
		if parsed, err := time.Parse(time.RFC3339, raw); err == nil {
			end = parsed
		}
	}
	return start, end
}

func normalizeCustomParams(params map[string]string) map[string]string {
	if len(params) == 0 {
		return map[string]string{}
	}
	normalized := make(map[string]string, len(params))
	for key, value := range params {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		normalized[key] = value
	}
	return normalized
}

func setQueryValue(values url.Values, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		values.Set(key, value)
	}
}

func respectsDNT(r *http.Request) bool {
	return r.Header.Get("DNT") == "1" || r.Header.Get("Sec-GPC") == "1"
}

func stringPtrIfNotEmpty(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func intPtrIfPositive(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func (h *handler) ensureStore(w http.ResponseWriter) bool {
	if h.ctx.Store == nil {
		http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
		return false
	}
	return true
}

func writeJSON(ctx context.Context, w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.MarshalWrite(w, value); err != nil {
		shared.LoggerFromContext(ctx).Error("Failed to encode response", "error", err)
	}
}
