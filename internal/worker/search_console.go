package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/database"
	"hitkeep/internal/searchconsole"
)

type SearchConsoleSyncWorker struct {
	tenantMgr *database.TenantStoreManager
	source    searchconsole.Client
	now       func() time.Time
	interval  time.Duration
	runLimit  int
}

type SearchConsoleSyncRunSummary struct {
	Attempted int
	Succeeded int
	Failed    int
}

func NewSearchConsoleSyncWorker(tenantMgr *database.TenantStoreManager, source searchconsole.Client) *SearchConsoleSyncWorker {
	return &SearchConsoleSyncWorker{tenantMgr: tenantMgr, source: source, now: time.Now, interval: 15 * time.Minute, runLimit: 25}
}

func (w *SearchConsoleSyncWorker) Start(ctx context.Context) {
	if w == nil || w.tenantMgr == nil || w.source == nil {
		slog.Debug("Search Console sync worker is not configured")
		return
	}
	interval, limit := w.startConfig()
	slog.Info("Search Console sync worker enabled", "interval", interval.String(), "limit", limit)
	w.runDueAndLog(ctx, limit, "Initial Search Console sync run")
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runDueAndLog(ctx, limit, "Search Console sync run")
		}
	}
}

func (w *SearchConsoleSyncWorker) startConfig() (time.Duration, int) {
	interval := w.interval
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	limit := w.runLimit
	if limit <= 0 {
		limit = 25
	}
	return interval, limit
}

func (w *SearchConsoleSyncWorker) runDueAndLog(ctx context.Context, limit int, label string) {
	summary, err := w.RunDue(ctx, limit)
	if err != nil {
		slog.Error(label+" failed", "error", err)
		return
	}
	if summary.Attempted > 0 {
		slog.Info(label+" completed", "attempted", summary.Attempted, "succeeded", summary.Succeeded, "failed", summary.Failed)
	}
}

func (w *SearchConsoleSyncWorker) RunDue(ctx context.Context, limit int) (SearchConsoleSyncRunSummary, error) {
	if w == nil || w.tenantMgr == nil || w.source == nil {
		return SearchConsoleSyncRunSummary{}, fmt.Errorf("search console sync worker is not configured")
	}
	candidates, err := w.tenantMgr.Shared().ListGoogleSearchConsoleSyncCandidates(ctx, w.now().UTC(), limit)
	if err != nil {
		return SearchConsoleSyncRunSummary{}, err
	}
	var summary SearchConsoleSyncRunSummary
	for _, candidate := range candidates {
		summary.Attempted++
		if err := w.ImportSite(ctx, candidate.SiteID); err != nil {
			summary.Failed++
			slog.Warn("Search Console site sync failed",
				"site_id", candidate.SiteID,
				"team_id", candidate.TeamID,
				"category", searchconsole.ClassifyError(err),
			)
			continue
		}
		summary.Succeeded++
	}
	return summary, nil
}

func (w *SearchConsoleSyncWorker) ImportSite(ctx context.Context, siteID uuid.UUID) error {
	if w == nil || w.tenantMgr == nil || w.source == nil {
		return fmt.Errorf("search console sync worker is not configured")
	}
	shared := w.tenantMgr.Shared()
	mapping, err := shared.GetGoogleSearchConsoleSiteMapping(ctx, siteID)
	if err != nil {
		return err
	}
	if mapping == nil {
		return fmt.Errorf("search console mapping not found for site %s", siteID)
	}
	conn, err := shared.GetGoogleSearchConsoleConnection(ctx, mapping.TeamID)
	if err != nil {
		return err
	}
	if conn == nil || !conn.Connected {
		err := searchconsole.ClassifiedError(searchconsole.CategoryAuthorizationRevoked, fmt.Errorf("google search console connection is not active"))
		return w.recordSyncFailure(ctx, *mapping, err)
	}

	if err := appendSearchConsoleSyncStartedAudit(ctx, shared, *mapping); err != nil {
		return err
	}
	tenantStore, _, err := w.tenantMgr.ResolveSiteStore(ctx, siteID)
	if err != nil {
		return err
	}

	now := w.now().UTC()
	state, err := shared.GetGoogleSearchConsoleSyncState(ctx, siteID)
	if err != nil {
		return err
	}
	windows := searchConsoleSyncWindows(now, state)
	var rowsToImport []searchconsole.SearchAnalyticsRow
	for _, window := range windows {
		query := searchConsoleSyncQuery(mapping.PropertyURI, window)
		rows, err := w.source.QuerySearchAnalytics(ctx, googleSearchConsoleToken(conn), query)
		if err != nil {
			return w.recordSyncFailure(ctx, *mapping, err)
		}
		rowsToImport = append(rowsToImport, rows...)
	}
	importedRows := len(rowsToImport)
	if err := appendSearchConsoleSyncPreparedAudit(ctx, shared, *mapping, importedRows); err != nil {
		return err
	}
	if err := importSearchConsoleRows(ctx, tenantStore, rowsToImport, siteID, mapping.PropertyURI, now); err != nil {
		return w.recordSyncFailure(ctx, *mapping, err)
	}
	importedStart, importedEnd := mergedSearchConsoleImportedRange(state, windows)
	successState := database.GoogleSearchConsoleSyncStateInput{
		SiteID:            siteID,
		TeamID:            mapping.TeamID,
		State:             "succeeded",
		ImportedStartDate: importedStart,
		ImportedEndDate:   importedEnd,
		LastSuccessAt:     &now,
		LastAttemptAt:     &now,
		Manual:            false,
	}
	successAudit := searchConsoleSyncAuditParams(ctx, shared, *mapping, importedRows)
	if err := shared.UpsertGoogleSearchConsoleSyncStateWithAudit(ctx, successState, successAudit); err != nil {
		return err
	}
	return nil
}

type searchConsoleSyncWindow struct {
	Start time.Time
	End   time.Time
}

func searchConsoleSyncWindows(now time.Time, state *database.GoogleSearchConsoleSyncState) []searchConsoleSyncWindow {
	end := searchConsoleDate(now.AddDate(0, 0, -2))
	if state != nil && state.LastSuccessAt != nil {
		return []searchConsoleSyncWindow{{Start: end.AddDate(0, 0, -6), End: end}}
	}
	start := end.AddDate(0, 0, -89)
	return []searchConsoleSyncWindow{{Start: start, End: end}}
}

func mergedSearchConsoleImportedRange(state *database.GoogleSearchConsoleSyncState, windows []searchConsoleSyncWindow) (*time.Time, *time.Time) {
	if len(windows) == 0 {
		return nil, nil
	}
	start := windows[0].Start
	end := windows[len(windows)-1].End
	if state != nil {
		if state.ImportedStartDate != nil && state.ImportedStartDate.Before(start) {
			start = *state.ImportedStartDate
		}
		if state.ImportedEndDate != nil && state.ImportedEndDate.After(end) {
			end = *state.ImportedEndDate
		}
	}
	return &start, &end
}

func searchConsoleSyncQuery(propertyURI string, window searchConsoleSyncWindow) searchconsole.SearchAnalyticsQuery {
	return searchconsole.SearchAnalyticsQuery{
		SiteURL:         propertyURI,
		StartDate:       window.Start,
		EndDate:         window.End,
		Dimensions:      []string{"date", "query", "page", "country", "device"},
		DataState:       searchconsole.DataStateFinal,
		AggregationType: "auto",
		RowLimit:        25000,
	}
}

func importSearchConsoleRows(ctx context.Context, tenantStore *database.Store, rows []searchconsole.SearchAnalyticsRow, siteID uuid.UUID, propertyURI string, importedAt time.Time) error {
	inputs := make([]database.SearchConsoleFactInput, 0, len(rows))
	for _, row := range rows {
		inputs = append(inputs, database.SearchConsoleFactInput{
			SiteID:          siteID,
			PropertyURI:     propertyURI,
			Date:            searchConsoleDate(row.Date),
			Query:           row.Query,
			Page:            row.Page,
			Country:         row.Country,
			Device:          row.Device,
			Clicks:          row.Clicks,
			Impressions:     row.Impressions,
			CTR:             row.CTR,
			Position:        row.Position,
			AggregationType: row.AggregationType,
			DataState:       row.DataState,
			ImportedAt:      importedAt,
		})
	}
	return tenantStore.UpsertSearchConsoleFacts(ctx, inputs)
}

func (w *SearchConsoleSyncWorker) recordSyncFailure(ctx context.Context, mapping database.GoogleSearchConsoleSiteMapping, syncErr error) error {
	now := w.now().UTC()
	category := searchconsole.ClassifyError(syncErr)
	nextRetry := searchConsoleRetryTime(now, category)
	previous, err := w.tenantMgr.Shared().GetGoogleSearchConsoleSyncState(ctx, mapping.SiteID)
	if err != nil {
		return err
	}
	input := database.GoogleSearchConsoleSyncStateInput{
		SiteID:            mapping.SiteID,
		TeamID:            mapping.TeamID,
		State:             searchConsoleFailureState(category),
		LastAttemptAt:     &now,
		LastErrorCategory: string(category),
		NextRetryAt:       nextRetry,
		Manual:            false,
	}
	if previous != nil {
		input.ImportedStartDate = previous.ImportedStartDate
		input.ImportedEndDate = previous.ImportedEndDate
		input.LastSuccessAt = previous.LastSuccessAt
	}
	failureAudit := searchConsoleSyncFailureAuditParams(ctx, w.tenantMgr.Shared(), mapping, category)
	if err := w.tenantMgr.Shared().UpsertGoogleSearchConsoleSyncStateWithAudit(ctx, input, failureAudit); err != nil {
		return err
	}
	return syncErr
}

func searchConsoleRetryTime(now time.Time, category searchconsole.ErrorCategory) *time.Time {
	var retry time.Time
	switch category {
	case searchconsole.CategoryQuotaLimited:
		retry = now.Add(6 * time.Hour)
	case searchconsole.CategoryGoogleUnavailable, searchconsole.CategoryUnknown:
		retry = now.Add(time.Hour)
	case searchconsole.CategoryAuthorizationRevoked, searchconsole.CategoryTokenRefreshFailed, searchconsole.CategoryPropertyAccessLost, searchconsole.CategoryCredentialsInvalid, searchconsole.CategoryCredentialsMissing, searchconsole.CategoryAPIDisabled:
		return nil
	}
	return &retry
}

func searchConsoleFailureState(category searchconsole.ErrorCategory) string {
	switch category {
	case searchconsole.CategoryAuthorizationRevoked, searchconsole.CategoryTokenRefreshFailed, searchconsole.CategoryPropertyAccessLost, searchconsole.CategoryCredentialsInvalid, searchconsole.CategoryCredentialsMissing, searchconsole.CategoryAPIDisabled:
		return "needs_attention"
	case searchconsole.CategoryQuotaLimited, searchconsole.CategoryGoogleUnavailable, searchconsole.CategoryUnknown:
		return "failed"
	}
	return "failed"
}

func googleSearchConsoleToken(conn *database.GoogleSearchConsoleConnection) searchconsole.Token {
	if conn == nil {
		return searchconsole.Token{}
	}
	return searchconsole.Token{
		AccessToken:        conn.AccessToken,
		RefreshToken:       conn.RefreshToken,
		TokenType:          conn.TokenType,
		Scope:              conn.Scope,
		Expiry:             conn.TokenExpiry,
		GoogleAccountEmail: conn.GoogleAccountEmail,
		GoogleAccountID:    conn.GoogleAccountID,
	}
}

func searchConsoleDate(value time.Time) time.Time {
	year, month, day := value.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func searchConsoleSyncAuditParams(ctx context.Context, shared *database.Store, mapping database.GoogleSearchConsoleSiteMapping, importedRows int) database.AuditEntryParams {
	targetLabel := ""
	if site, err := shared.GetSiteByID(ctx, mapping.SiteID); err == nil && site != nil {
		targetLabel = site.Domain
	}
	return database.AuditEntryParams{
		TeamID:      mapping.TeamID,
		Action:      "google_search_console.sync_imported",
		TargetType:  "site",
		TargetID:    mapping.SiteID.String(),
		TargetLabel: targetLabel,
		Outcome:     "success",
		Details:     fmt.Sprintf("outcome=imported;property_uri=%s;imported_rows=%d", mapping.PropertyURI, importedRows),
	}
}

func appendSearchConsoleSyncPreparedAudit(ctx context.Context, shared *database.Store, mapping database.GoogleSearchConsoleSiteMapping, preparedRows int) error {
	targetLabel := ""
	if site, err := shared.GetSiteByID(ctx, mapping.SiteID); err == nil && site != nil {
		targetLabel = site.Domain
	}
	if err := shared.AppendAuditEntry(ctx, database.AuditEntryParams{
		TeamID:      mapping.TeamID,
		Action:      "google_search_console.sync_import_prepared",
		TargetType:  "site",
		TargetID:    mapping.SiteID.String(),
		TargetLabel: targetLabel,
		Outcome:     "success",
		Details:     fmt.Sprintf("outcome=prepared;property_uri=%s;prepared_rows=%d", strings.TrimSpace(mapping.PropertyURI), preparedRows),
	}); err != nil {
		return fmt.Errorf("append Search Console sync import prepared audit: %w", err)
	}
	return nil
}

func appendSearchConsoleSyncStartedAudit(ctx context.Context, shared *database.Store, mapping database.GoogleSearchConsoleSiteMapping) error {
	targetLabel := ""
	if site, err := shared.GetSiteByID(ctx, mapping.SiteID); err == nil && site != nil {
		targetLabel = site.Domain
	}
	if err := shared.AppendAuditEntry(ctx, database.AuditEntryParams{
		TeamID:      mapping.TeamID,
		Action:      "google_search_console.sync_started",
		TargetType:  "site",
		TargetID:    mapping.SiteID.String(),
		TargetLabel: targetLabel,
		Outcome:     "success",
		Details:     fmt.Sprintf("outcome=started;property_uri=%s", strings.TrimSpace(mapping.PropertyURI)),
	}); err != nil {
		return fmt.Errorf("append Search Console sync start audit: %w", err)
	}
	return nil
}

func searchConsoleSyncFailureAuditParams(ctx context.Context, shared *database.Store, mapping database.GoogleSearchConsoleSiteMapping, category searchconsole.ErrorCategory) database.AuditEntryParams {
	targetLabel := ""
	if site, err := shared.GetSiteByID(ctx, mapping.SiteID); err == nil && site != nil {
		targetLabel = site.Domain
	}
	return database.AuditEntryParams{
		TeamID:      mapping.TeamID,
		Action:      "google_search_console.sync_failed",
		TargetType:  "site",
		TargetID:    mapping.SiteID.String(),
		TargetLabel: targetLabel,
		Outcome:     "failure",
		Details:     fmt.Sprintf("outcome=failed;property_uri=%s;category=%s", mapping.PropertyURI, category),
	}
}
