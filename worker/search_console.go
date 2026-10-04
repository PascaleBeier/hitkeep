package worker

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"hitkeep/database"
	"hitkeep/hklog"
	"hitkeep/searchconsole"
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

const (
	searchConsoleSyncStageConnection    = "connection"
	searchConsoleSyncStageQuery         = "query"
	searchConsoleSyncStageImport        = "import"
	searchConsoleSyncStageOrchestration = "orchestration"
)

type searchConsoleSyncError struct {
	stage string
	err   error
}

func (e *searchConsoleSyncError) Error() string {
	if e == nil || e.err == nil {
		return "google search console sync failed"
	}
	return e.err.Error()
}

func (e *searchConsoleSyncError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func SearchConsoleSyncLogValues(err error) []any {
	diagnostic := searchconsole.DiagnoseError(err)
	values := []any{
		"stage", searchConsoleSyncErrorStage(err),
		"category", diagnostic.Category,
		"error_message", diagnostic.Message,
	}
	if diagnostic.HTTPStatus > 0 {
		values = append(values, "http_status", diagnostic.HTTPStatus)
	}
	if diagnostic.ProviderReason != "" {
		values = append(values, "provider_reason", diagnostic.ProviderReason)
	}
	return values
}

func searchConsoleSyncErrorStage(err error) string {
	var syncErr *searchConsoleSyncError
	if errors.As(err, &syncErr) && strings.TrimSpace(syncErr.stage) != "" {
		return syncErr.stage
	}
	return searchConsoleSyncStageOrchestration
}

func NewSearchConsoleSyncWorker(tenantMgr *database.TenantStoreManager, source searchconsole.Client) *SearchConsoleSyncWorker {
	return &SearchConsoleSyncWorker{tenantMgr: tenantMgr, source: source, now: time.Now, interval: 15 * time.Minute, runLimit: 25}
}

func (w *SearchConsoleSyncWorker) Start(ctx context.Context) {
	if w == nil || w.tenantMgr == nil || w.source == nil {
		hklog.LoggerFromContext(ctx).Debug("Search Console sync worker is not configured")
		return
	}
	interval, limit := w.startConfig()
	hklog.LoggerFromContext(ctx).Info("Search Console sync worker enabled", "interval", interval.String(), "limit", limit)
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
		hklog.LoggerFromContext(ctx).Error(label+" failed", SearchConsoleSyncLogValues(err)...)
		return
	}
	if summary.Attempted > 0 {
		hklog.LoggerFromContext(ctx).Info(label+" completed", "attempted", summary.Attempted, "succeeded", summary.Succeeded, "failed", summary.Failed)
	}
}

const (
	searchConsoleRunDueTimeout        = time.Minute
	searchConsoleFailureRecordTimeout = 10 * time.Second
)

func (w *SearchConsoleSyncWorker) RunDue(ctx context.Context, limit int) (SearchConsoleSyncRunSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, searchConsoleRunDueTimeout)
	defer cancel()
	if w == nil || w.tenantMgr == nil || w.source == nil {
		return SearchConsoleSyncRunSummary{}, fmt.Errorf("search console sync worker is not configured")
	}
	now := w.now().UTC()
	pruned, pruneErr := w.tenantMgr.Shared().PruneGoogleSearchConsoleErrorMessages(ctx, now.Add(-database.GoogleSearchConsoleErrorMessageRetention))
	if pruneErr != nil {
		hklog.LoggerFromContext(ctx).Error("Failed to prune retained Google Search Console error messages", "error", pruneErr)
	} else if pruned > 0 {
		hklog.LoggerFromContext(ctx).Info("Pruned retained Google Search Console error messages", "audit_entries", pruned)
	}
	candidates, err := w.tenantMgr.Shared().ListGoogleSearchConsoleSyncCandidates(ctx, now, limit)
	if err != nil {
		return SearchConsoleSyncRunSummary{}, err
	}
	var summary SearchConsoleSyncRunSummary
	for _, candidate := range candidates {
		summary.Attempted++
		if err := w.ImportSite(ctx, candidate.SiteID); err != nil {
			summary.Failed++
			logValues := make([]any, 0, 14)
			logValues = append(logValues,
				"site_id", candidate.SiteID,
				"team_id", candidate.TeamID,
			)
			logValues = append(logValues, SearchConsoleSyncLogValues(err)...)
			hklog.LoggerFromContext(ctx).Warn("Search Console site sync failed", logValues...)
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
		return w.recordSyncFailure(ctx, *mapping, searchConsoleSyncStageConnection, err)
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
	days := searchConsoleSyncDays(windows)
	// The first sync, or a manual one, brings every day in the initial range up
	// to date once per property. After that, manual syncs skip the older days.
	totalsComplete := state != nil && state.TotalsBackfilledPropertyURI == mapping.PropertyURI
	if !totalsComplete && (state == nil || state.LastSuccessAt == nil || state.Manual) {
		days, err = searchConsoleCatchUpDays(ctx, tenantStore, siteID, mapping.PropertyURI, now)
		if err != nil {
			return err
		}
		totalsComplete = true
	}
	token := googleSearchConsoleToken(conn)
	importedRows := 0
	for _, window := range days {
		// Replace the day only after every request succeeds.
		queries := searchConsoleSyncQueries(mapping.PropertyURI, window)
		results := make([][]searchconsole.SearchAnalyticsRow, len(queries))
		group, groupCtx := errgroup.WithContext(ctx)
		for i, query := range queries {
			group.Go(func() (err error) {
				results[i], err = w.source.QuerySearchAnalytics(groupCtx, token, query)
				return err
			})
		}
		if err := group.Wait(); err != nil {
			return w.recordSyncFailure(ctx, *mapping, searchConsoleSyncStageQuery, err)
		}
		rows := slices.Concat(results...)
		if err := importSearchConsoleRows(ctx, tenantStore, rows, siteID, mapping.PropertyURI, window, now); err != nil {
			return w.recordSyncFailure(ctx, *mapping, searchConsoleSyncStageImport, err)
		}
		importedRows += len(rows)
	}
	if err := appendSearchConsoleSyncPreparedAudit(ctx, shared, *mapping, importedRows); err != nil {
		return err
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
	if totalsComplete {
		successState.TotalsBackfilledPropertyURI = mapping.PropertyURI
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
	if state != nil && state.LastSuccessAt != nil {
		return []searchConsoleSyncWindow{searchConsoleRecentWindow(now)}
	}
	return []searchConsoleSyncWindow{searchConsoleInitialWindow(now)}
}

func searchConsoleRecentWindow(now time.Time) searchConsoleSyncWindow {
	end := searchConsoleDate(now.AddDate(0, 0, -2))
	return searchConsoleSyncWindow{Start: end.AddDate(0, 0, -6), End: end}
}

func searchConsoleInitialWindow(now time.Time) searchConsoleSyncWindow {
	end := searchConsoleDate(now.AddDate(0, 0, -2))
	return searchConsoleSyncWindow{Start: end.AddDate(0, 0, -89), End: end}
}

func searchConsoleSyncDays(windows []searchConsoleSyncWindow) []searchConsoleSyncWindow {
	var days []searchConsoleSyncWindow
	for _, window := range windows {
		start := searchConsoleDate(window.Start)
		end := searchConsoleDate(window.End)
		for day := end; !day.Before(start); day = day.AddDate(0, 0, -1) {
			days = append(days, searchConsoleSyncWindow{Start: day, End: day})
		}
	}
	return days
}

// searchConsoleCatchUpDays returns the recent window's days, then every older
// day in the initial range without audience totals, newest first. Each day
// commits on its own, so a sync cut short skips the finished days next time.
// Days with no traffic never gain totals and are fetched again until a sync
// completes and records the backfill.
func searchConsoleCatchUpDays(ctx context.Context, tenantStore *database.Store, siteID uuid.UUID, propertyURI string, now time.Time) ([]searchConsoleSyncWindow, error) {
	recent := searchConsoleRecentWindow(now)
	older := searchConsoleSyncWindow{Start: searchConsoleInitialWindow(now).Start, End: recent.Start.AddDate(0, 0, -1)}
	withTotals, err := tenantStore.ListSearchConsoleAudienceTotalsDays(ctx, siteID, propertyURI, older.Start, older.End)
	if err != nil {
		return nil, err
	}
	done := make(map[string]bool, len(withTotals))
	for _, day := range withTotals {
		done[day.Format(time.DateOnly)] = true
	}
	days := searchConsoleSyncDays([]searchConsoleSyncWindow{recent})
	for _, day := range searchConsoleSyncDays([]searchConsoleSyncWindow{older}) {
		if !done[day.Start.Format(time.DateOnly)] {
			days = append(days, day)
		}
	}
	return days, nil
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

// searchConsoleSyncQueries returns one day's requests. Google omits anonymized
// searches from rows grouped by query, and from rows grouped by page together
// with country or device, so page and audience totals are separate requests
// next to the query rows.
func searchConsoleSyncQueries(propertyURI string, window searchConsoleSyncWindow) []searchconsole.SearchAnalyticsQuery {
	dimensionSets := [][]string{
		{"date", "query", "page", "country", "device"},
		{"date", "page"},
		{"date", "country", "device"},
	}
	queries := make([]searchconsole.SearchAnalyticsQuery, 0, len(dimensionSets))
	for _, dimensions := range dimensionSets {
		queries = append(queries, searchconsole.SearchAnalyticsQuery{
			SiteURL:         propertyURI,
			StartDate:       window.Start,
			EndDate:         window.End,
			Dimensions:      dimensions,
			DataState:       searchconsole.DataStateFinal,
			AggregationType: "auto",
			RowLimit:        25000,
		})
	}
	return queries
}

func importSearchConsoleRows(ctx context.Context, tenantStore *database.Store, rows []searchconsole.SearchAnalyticsRow, siteID uuid.UUID, propertyURI string, window searchConsoleSyncWindow, importedAt time.Time) error {
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
	return tenantStore.ReplaceSearchConsoleFacts(ctx, database.SearchConsoleFactScope{
		SiteID: siteID, PropertyURI: propertyURI, StartDate: window.Start, EndDate: window.End, DataState: searchconsole.DataStateFinal,
	}, inputs)
}

func (w *SearchConsoleSyncWorker) recordSyncFailure(ctx context.Context, mapping database.GoogleSearchConsoleSiteMapping, stage string, syncErr error) error {
	// The sync often fails because ctx expired or was canceled; the failure
	// must still be recorded so the retry backs off and drops the manual flag.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), searchConsoleFailureRecordTimeout)
	defer cancel()
	now := w.now().UTC()
	diagnostic := searchconsole.DiagnoseError(syncErr)
	category := diagnostic.Category
	nextRetry := searchConsoleRetryTime(now, category)
	previous, err := w.tenantMgr.Shared().GetGoogleSearchConsoleSyncState(ctx, mapping.SiteID)
	if err != nil {
		return &searchConsoleSyncError{stage: stage, err: fmt.Errorf("load previous sync state while recording failure: %w", err)}
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
	failureAudit := searchConsoleSyncFailureAuditParams(ctx, w.tenantMgr.Shared(), mapping, stage, diagnostic)
	if err := w.tenantMgr.Shared().UpsertGoogleSearchConsoleSyncStateWithAudit(ctx, input, failureAudit); err != nil {
		return &searchConsoleSyncError{stage: stage, err: fmt.Errorf("record sync failure: %w", err)}
	}
	return &searchConsoleSyncError{stage: stage, err: syncErr}
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

func searchConsoleSyncFailureAuditParams(ctx context.Context, shared *database.Store, mapping database.GoogleSearchConsoleSiteMapping, stage string, diagnostic searchconsole.ErrorDiagnostic) database.AuditEntryParams {
	targetLabel := ""
	if site, err := shared.GetSiteByID(ctx, mapping.SiteID); err == nil && site != nil {
		targetLabel = site.Domain
	}
	details := fmt.Sprintf(
		"outcome=failed;property_uri=%s;stage=%s;category=%s",
		strings.TrimSpace(mapping.PropertyURI),
		strings.TrimSpace(stage),
		diagnostic.Category,
	)
	if diagnostic.HTTPStatus > 0 {
		details += fmt.Sprintf(";http_status=%d", diagnostic.HTTPStatus)
	}
	if diagnostic.ProviderReason != "" {
		details += ";provider_reason=" + strconv.Quote(diagnostic.ProviderReason)
	}
	if diagnostic.Message != "" {
		details += ";error_message=" + strconv.Quote(diagnostic.Message)
	}
	return database.AuditEntryParams{
		TeamID:      mapping.TeamID,
		Action:      "google_search_console.sync_failed",
		TargetType:  "site",
		TargetID:    mapping.SiteID.String(),
		TargetLabel: targetLabel,
		Outcome:     "failure",
		Details:     details,
	}
}
