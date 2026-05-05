package admin

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/worker"
)

type nsqPinger interface {
	Ping() error
}

func (h *handler) handleGetSystem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := h.ctx.Config

		uptime := time.Since(h.ctx.StartedAt).String()
		info := api.SystemInfo{
			Version:         cfg.Version,
			RuntimeMode:     systemRuntimeMode(cfg),
			Uptime:          uptime,
			PublicURL:       cfg.PublicURL,
			EnabledFeatures: systemFeatureStatuses(cfg, h.ctx.Mailer != nil),
			ConfigFlags:     map[string]any{},
		}

		writeJSON(w, http.StatusOK, info)
	}
}

func systemRuntimeMode(cfg *config.Config) string {
	if cfg.CloudHosted {
		return "cloud"
	}
	return "oss"
}

func systemFeatureStatuses(cfg *config.Config, mailerConfigured bool) []api.SystemFeatureStatus {
	billingEnabled := cfg.CloudHosted && strings.TrimSpace(cfg.StripeSecretKey) != ""
	searchConsoleEnabled := googleSearchConsoleCredentialsConfigured(cfg)

	return []api.SystemFeatureStatus{
		{Key: "mcp", Enabled: cfg.MCPEnabled, Detail: enabledDetail(cfg.MCPEnabled, cfg.MCPPath)},
		{Key: "mcp_docs", Enabled: cfg.MCPEnabled && cfg.MCPDocsEnabled, Detail: enabledDetail(cfg.MCPEnabled && cfg.MCPDocsEnabled, cfg.MCPDocsURL)},
		{Key: "automatic_backups", Enabled: cfg.BackupPath != "", Detail: backupFeatureDetail(cfg)},
		{Key: "spam_auto_update", Enabled: cfg.SpamFilterAutoUpdate, Detail: enabledDetail(cfg.SpamFilterAutoUpdate, formatFeatureInterval(cfg.SpamFilterUpdateIntervalMin))},
		{Key: "mail_delivery", Enabled: mailerConfigured, Detail: mailFeatureDetail(cfg, mailerConfigured)},
		{Key: "google_search_console", Enabled: searchConsoleEnabled, Detail: enabledDetail(searchConsoleEnabled, "oauth")},
		{Key: "managed_cloud", Enabled: cfg.CloudHosted, Detail: cloudFeatureDetail(cfg)},
		{Key: "cloud_signup", Enabled: cfg.CloudHosted && cfg.CloudSignupEnabled},
		{Key: "billing", Enabled: billingEnabled, Detail: enabledDetail(billingEnabled, "stripe")},
	}
}

func backupFeatureDetail(cfg *config.Config) string {
	if cfg.BackupPath == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.BackupPath)), "s3://") {
		return "s3"
	}
	return "local"
}

func mailFeatureDetail(cfg *config.Config, mailerConfigured bool) string {
	if !mailerConfigured {
		return ""
	}
	detail := strings.TrimSpace(cfg.MailDriver)
	if detail != "" {
		return detail
	}
	return "smtp"
}

func cloudFeatureDetail(cfg *config.Config) string {
	if !cfg.CloudHosted {
		return ""
	}
	if detail := strings.TrimSpace(cfg.CloudPlanName); detail != "" {
		return detail
	}
	return strings.TrimSpace(cfg.CloudPlanCode)
}

func enabledDetail(enabled bool, detail string) string {
	if !enabled {
		return ""
	}
	return strings.TrimSpace(detail)
}

func formatFeatureInterval(minutes int) string {
	if minutes <= 0 {
		return ""
	}
	if minutes%1440 == 0 {
		return fmt.Sprintf("%dd", minutes/1440)
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%dh", minutes/60)
	}
	return fmt.Sprintf("%dm", minutes)
}

func googleSearchConsoleCredentialsConfigured(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return strings.TrimSpace(cfg.GoogleSearchConsoleClientID) != "" && strings.TrimSpace(cfg.GoogleSearchConsoleClientSecret) != ""
}

func (h *handler) handleGetHealth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		health := api.SystemHealth{
			Status:   "healthy",
			Database: "ok",
			Workers:  "ok",
			IsLeader: true,
		}

		if h.ctx.Cluster != nil {
			health.IsLeader = h.ctx.Cluster.IsLeader()
		}
		if workers, ok := workerHealthStatus(health.IsLeader, h.ctx.Producer); ok {
			health.Workers = workers
		} else {
			health.Workers = workers
			health.Status = "degraded"
		}

		if h.ctx.Store != nil {
			if err := h.ctx.Store.DB().Ping(); err != nil {
				health.Database = fmt.Sprintf("error: %v", err)
				health.Status = "degraded"
			}
		}

		writeJSON(w, http.StatusOK, health)
	}
}

func (h *handler) handleGetSearchConsole() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		storeStatus, err := h.ctx.Store.GetGoogleSearchConsoleSystemStatus(r.Context())
		if err != nil {
			slog.Error("Failed to read Google Search Console system status", "error", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		status := googleSearchConsoleSystemStatus(h.ctx.Config, h.ctx.TenantStores != nil, storeStatus)
		writeJSON(w, http.StatusOK, status)
	}
}

func googleSearchConsoleSystemStatus(cfg *config.Config, tenantStoresConfigured bool, storeStatus database.GoogleSearchConsoleSystemStatus) api.SystemSearchConsoleStatus {
	credentialsStatus := searchConsoleCredentialsStatus(cfg)
	workerStatus := searchConsoleWorkerStatus(credentialsStatus, tenantStoresConfigured)
	syncStatus := searchConsoleSystemSyncStatus(storeStatus)

	return api.SystemSearchConsoleStatus{
		Status:              searchConsoleOverallStatus(credentialsStatus, workerStatus, syncStatus, storeStatus.MappedSites),
		CredentialsStatus:   credentialsStatus,
		WorkerStatus:        workerStatus,
		SyncStatus:          syncStatus,
		ConnectedTeams:      storeStatus.ConnectedTeams,
		MappedSites:         storeStatus.MappedSites,
		PendingSyncs:        storeStatus.PendingSyncs,
		RunningSyncs:        storeStatus.RunningSyncs,
		FailedSyncs:         storeStatus.FailedSyncs,
		NeedsAttentionSyncs: storeStatus.NeedsAttentionSyncs,
		LastSuccessAt:       storeStatus.LastSuccessAt,
		LastAttemptAt:       storeStatus.LastAttemptAt,
		NextRetryAt:         storeStatus.NextRetryAt,
	}
}

func searchConsoleCredentialsStatus(cfg *config.Config) string {
	if googleSearchConsoleCredentialsConfigured(cfg) {
		return "configured"
	}
	return "missing"
}

func searchConsoleWorkerStatus(credentialsStatus string, tenantStoresConfigured bool) string {
	if credentialsStatus == "configured" && tenantStoresConfigured {
		return "enabled"
	}
	return "disabled"
}

func searchConsoleSystemSyncStatus(status database.GoogleSearchConsoleSystemStatus) string {
	if status.NeedsAttentionSyncs > 0 {
		return "needs_attention"
	}
	if status.FailedSyncs > 0 {
		return "failed"
	}
	if status.RunningSyncs > 0 {
		return "running"
	}
	if status.PendingSyncs > 0 {
		return "pending"
	}
	if status.MappedSites > 0 {
		return "healthy"
	}
	return "idle"
}

func searchConsoleOverallStatus(credentialsStatus, workerStatus, syncStatus string, mappedSites int) string {
	if credentialsStatus != "configured" {
		return "not_configured"
	}
	if workerStatus != "enabled" || syncStatus == "failed" {
		return "degraded"
	}
	if syncStatus == "needs_attention" {
		return "needs_attention"
	}
	if syncStatus == "pending" || syncStatus == "running" {
		return "syncing"
	}
	if mappedSites == 0 {
		return "idle"
	}
	return "healthy"
}

func workerHealthStatus(isLeader bool, producer nsqPinger) (string, bool) {
	if !isLeader {
		return "standby", true
	}
	if producer == nil || (reflect.ValueOf(producer).Kind() == reflect.Pointer && reflect.ValueOf(producer).IsNil()) {
		return "unavailable", false
	}
	if err := producer.Ping(); err != nil {
		if errors.Is(err, context.Canceled) {
			return "stopping", false
		}
		return fmt.Sprintf("error: %v", err), false
	}
	return "ok", true
}

func (h *handler) handleGetStorage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		cfg := h.ctx.Config

		storage := api.SystemStorage{
			SharedDBPath: cfg.DBPath,
			DataPath:     cfg.DataPath,
			BackupPath:   cfg.BackupPath,
		}

		if fi, err := os.Stat(cfg.DBPath); err == nil {
			storage.SharedDBBytes = fi.Size()
		}

		tenants, err := h.ctx.Store.GetTenantList(ctx)
		if err == nil {
			storage.TenantDBCount = len(tenants)
			for i, t := range tenants {
				tenantPath := filepath.Join(cfg.DataPath, "tenants", t.TenantID.String(), "hitkeep.db")
				tenants[i].Path = tenantPath
				if fi, err := os.Stat(tenantPath); err == nil {
					tenants[i].Bytes = fi.Size()
				}
			}
			storage.TenantDBs = tenants
		}

		spamCachePath := cfg.SpamFilterPath
		if spamCachePath == "" {
			spamCachePath = cfg.DataPath + "/spam-filter.json"
		}
		storage.SpamCachePath = spamCachePath

		diskPath := strings.TrimSpace(cfg.DataPath)
		if diskPath == "" && strings.TrimSpace(cfg.DBPath) != "" {
			diskPath = filepath.Dir(cfg.DBPath)
		}
		if diskPath == "" {
			diskPath = "."
		}
		if available, total, err := filesystemUsage(diskPath); err == nil {
			storage.DiskAvailable = available
			storage.DiskTotal = total
		} else {
			slog.Debug("Failed to read filesystem usage", "path", diskPath, "error", err)
		}

		writeJSON(w, http.StatusOK, storage)
	}
}

func (h *handler) handleGetIngestStats() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		stats := api.SystemIngestStats{}

		since := time.Now().UTC().Add(-24 * time.Hour)

		if h.ctx.TenantStores != nil {
			counts, err := h.ctx.TenantStores.GetRecentIngestCounts(ctx, since)
			if err == nil {
				stats.RecentHits = counts.Hits
				stats.RecentEvents = counts.Events
			} else {
				slog.Warn("Failed to read tenant ingest counts", "error", err)
			}
		} else {
			counts, err := h.ctx.Store.GetRecentIngestCounts(ctx, since)
			if err == nil {
				stats.RecentHits = counts.Hits
				stats.RecentEvents = counts.Events
			} else {
				slog.Warn("Failed to read ingest counts", "error", err)
			}
		}

		if h.ctx.SystemCounters != nil {
			stats.RecentRejections = int(h.ctx.SystemCounters.Rejections.Load())
			stats.RecentSpam = int(h.ctx.SystemCounters.Spam.Load())
		}

		secs := time.Since(since).Seconds()
		if secs > 0 {
			stats.HitsPerSecond = float64(stats.RecentHits) / secs
		}

		writeJSON(w, http.StatusOK, stats)
	}
}

func (h *handler) handleGetBackups() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := api.SystemBackupStatus{}
		if h.ctx.BackupStatus != nil {
			status = h.ctx.BackupStatus.Status()
		}

		writeJSON(w, http.StatusOK, status)
	}
}

func (h *handler) handleGetSpamFilter() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := api.SystemSpamStatus{
			AutoUpdate: h.ctx.Config.SpamFilterAutoUpdate,
		}

		spamPath := h.ctx.Config.SpamFilterPath
		if spamPath == "" {
			spamPath = h.ctx.Config.DataPath + "/spam-filter.json"
		}
		status.DBPath = spamPath

		if h.ctx.SpamFilter != nil {
			status.RuleCount = h.ctx.SpamFilter.RuleCount()
			status.LastRefresh = h.ctx.SpamFilter.LastRefresh()
		}

		writeJSON(w, http.StatusOK, status)
	}
}

func (h *handler) handleGetImportStageCleanup() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.ctx.Store == nil {
			http.Error(w, "Service not available on this node", http.StatusServiceUnavailable)
			return
		}

		retentionDays := h.ctx.Config.ImportStageRetentionDays
		estimate := api.ImportStageCleanupEstimate{}
		if retentionDays > 0 {
			cleaner := worker.NewImportStageCleaner(h.ctx.Store, h.ctx.Config.DataPath, retentionDays)
			var err error
			estimate, err = cleaner.Estimate(r.Context())
			if err != nil {
				slog.Error("Failed to estimate import stage cleanup", "error", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
				return
			}
		}

		status := api.SystemImportStageCleanupStatus{
			Enabled:       retentionDays > 0,
			RetentionDays: retentionDays,
			StaleImports:  estimate.Imports,
			StaleFiles:    estimate.Files,
			StaleBytes:    estimate.Bytes,
		}
		if h.ctx.ImportStageCleanupStatus != nil {
			status = h.ctx.ImportStageCleanupStatus.Status(estimate)
		}

		writeJSON(w, http.StatusOK, status)
	}
}

func (h *handler) handleGetCaches() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := api.SystemCacheStatus{
			PermissionsCache: api.SystemCacheEntry{
				Size:    0,
				MaxSize: 8192,
				TTL:     "30s",
			},
			APIClientCache: api.SystemCacheEntry{
				Size:    0,
				MaxSize: 4096,
				TTL:     "5m",
			},
			RateLimiterCache: api.SystemCacheEntry{
				Size:    0,
				MaxSize: 10000,
				TTL:     "3m",
			},
			Status: "healthy",
		}

		if h.ctx.Store != nil {
			permsSize := h.ctx.Store.CacheInstanceRoleSize()
			apiSize := h.ctx.Store.CacheAPIClientAuthSize()
			status.PermissionsCache.Size = permsSize
			status.APIClientCache.Size = apiSize

			if float64(permsSize) > float64(8192)*0.9 || float64(apiSize) > float64(4096)*0.9 {
				status.Status = "pressure"
			}
		}

		if h.ctx.ApiLimiter != nil {
			status.RateLimiterCache.Size = h.ctx.ApiLimiter.Len()
		}

		writeJSON(w, http.StatusOK, status)
	}
}

func (h *handler) handleGetMail() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := h.ctx.Config
		status := api.SystemMailStatus{
			Configured:  cfg.MailHost != "" || cfg.MailDriver != "",
			Driver:      cfg.MailDriver,
			Host:        cfg.MailHost,
			Port:        cfg.MailPort,
			Encryption:  cfg.MailEncryption,
			FromAddress: cfg.MailFromAddress,
			FromName:    cfg.MailFromName,
			PasswordSet: cfg.MailPassword != "",
		}

		if cfg.MailUsername != "" {
			if len(cfg.MailUsername) > 4 {
				status.Username = cfg.MailUsername[:4] + "****"
			} else {
				status.Username = cfg.MailUsername[:1] + "****"
			}
		}

		if h.ctx.MailTestTracker != nil {
			status.LastTestAt, status.LastTestOK = h.ctx.MailTestTracker.Status()
		}

		writeJSON(w, http.StatusOK, status)
	}
}

func parseInstanceAuditFilter(q url.Values, includeOffset bool) (database.InstanceAuditFilter, error) {
	filter := database.InstanceAuditFilter{
		Action:     strings.TrimSpace(q.Get("action")),
		TargetType: strings.TrimSpace(q.Get("target_type")),
		Outcome:    strings.TrimSpace(q.Get("outcome")),
		Query:      strings.TrimSpace(q.Get("query")),
	}

	if actorIDStr := strings.TrimSpace(q.Get("actor_id")); actorIDStr != "" {
		actorID, err := uuid.Parse(actorIDStr)
		if err != nil {
			return filter, fmt.Errorf("invalid actor_id")
		}
		filter.ActorID = actorID
	}
	if fromStr := strings.TrimSpace(q.Get("from")); fromStr != "" {
		t, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			return filter, fmt.Errorf("invalid from date, expected RFC3339")
		}
		filter.From = t
	}
	if toStr := strings.TrimSpace(q.Get("to")); toStr != "" {
		t, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			return filter, fmt.Errorf("invalid to date, expected RFC3339")
		}
		filter.To = t
	}
	if limitStr := strings.TrimSpace(q.Get("limit")); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil || limit < 0 {
			return filter, fmt.Errorf("invalid limit")
		}
		filter.Limit = limit
	}
	if includeOffset {
		if offsetStr := strings.TrimSpace(q.Get("offset")); offsetStr != "" {
			offset, err := strconv.Atoi(offsetStr)
			if err != nil || offset < 0 {
				return filter, fmt.Errorf("invalid offset")
			}
			filter.Offset = offset
		}
	}

	return filter, nil
}

func (h *handler) handleListAudit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		filter, err := parseInstanceAuditFilter(r.URL.Query(), true)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		entries, total, err := h.ctx.Store.ListInstanceAuditEntries(ctx, filter)
		if err != nil {
			slog.Error("Failed to list instance audit entries", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		limit := filter.Limit
		if limit <= 0 || limit > database.MaxInstanceAuditListLimit {
			limit = database.DefaultInstanceAuditListLimit
		}
		offset := filter.Offset

		resp := api.InstanceAuditListResponse{
			Entries: entries,
			Total:   total,
			Limit:   limit,
			Offset:  offset,
			HasMore: offset+len(entries) < total,
		}

		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *handler) handleExportAudit() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		q := r.URL.Query()

		filter, err := parseInstanceAuditFilter(q, false)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		entries, err := h.ctx.Store.ExportInstanceAuditEntries(ctx, filter)
		if err != nil {
			slog.Error("Failed to export instance audit entries", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		format := strings.TrimSpace(q.Get("format"))
		if format == "csv" {
			w.Header().Set("Content-Type", "text/csv")
			w.Header().Set("Content-Disposition", "attachment; filename=instance-audit-export.csv")

			writer := csv.NewWriter(w)
			if err := writer.Write([]string{"id", "created_at", "actor_id", "team_id", "target_user_id", "actor_email", "actor_role", "action", "target_type", "target_id", "target_label", "outcome", "ip_address", "ip_country_code", "request_id", "user_agent", "details"}); err != nil {
				slog.Error("Failed to write instance audit CSV header", "error", err)
				return
			}

			for _, entry := range entries {
				actorID := ""
				if entry.ActorID != nil {
					actorID = entry.ActorID.String()
				}
				teamID := ""
				if entry.TeamID != nil {
					teamID = entry.TeamID.String()
				}
				targetUserID := ""
				if entry.TargetUserID != nil {
					targetUserID = entry.TargetUserID.String()
				}
				if err := writer.Write([]string{
					entry.ID.String(),
					entry.CreatedAt.Format(time.RFC3339),
					actorID,
					teamID,
					targetUserID,
					entry.ActorEmailSnapshot,
					entry.ActorRoleSnapshot,
					entry.Action,
					entry.TargetType,
					entry.TargetID,
					entry.TargetLabel,
					entry.Outcome,
					entry.IPAddress,
					entry.IPCountryCode,
					entry.RequestID,
					entry.UserAgent,
					entry.Details,
				}); err != nil {
					slog.Error("Failed to write instance audit CSV row", "error", err, "audit_id", entry.ID)
					return
				}
			}
			writer.Flush()
			if err := writer.Error(); err != nil {
				slog.Error("Failed to flush instance audit CSV", "error", err)
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=instance-audit-export.json")
		if err := json.NewEncoder(w).Encode(entries); err != nil {
			slog.Error("Failed to encode instance audit export", "error", err)
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode response", "error", err)
	}
}
