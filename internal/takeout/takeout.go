package takeout

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
	"hitkeep/internal/exportfmt"
)

type TakeoutService struct {
	store        *database.Store
	tenantStores *database.TenantStoreManager
	path         string
}

type ExportFile struct {
	File *os.File
	Info os.FileInfo
	Name string
}

func NewTakeoutService(store *database.Store, path string) *TakeoutService {
	return &TakeoutService{
		store: store,
		path:  path,
	}
}

func NewTakeoutServiceWithTenantStores(store *database.Store, tenantStores *database.TenantStoreManager, path string) *TakeoutService {
	return &TakeoutService{
		store:        store,
		tenantStores: tenantStores,
		path:         path,
	}
}

func (s *TakeoutService) ExportUserData(ctx context.Context, userID uuid.UUID, format string) (string, error) {
	// Ensure export directory exists
	if err := os.MkdirAll(s.path, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	sites, err := s.store.ListAccessibleSitesForTakeout(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve accessible sites: %w", err)
	}

	normalizedFormat := exportfmt.Normalize(format, exportfmt.FormatXLSX)
	filename := filepath.Join(s.path, fmt.Sprintf("user_takeout_%s_%d.%s", userID, time.Now().Unix(), normalizedFormat))

	if s.tenantStores != nil {
		sources, err := s.takeoutSourcesForSites(ctx, userID, sites)
		if err != nil {
			return "", err
		}
		return s.exportTakeoutFromSources(ctx, "user", filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), sources)
	}

	return s.exportTakeoutFromStore(ctx, s.store, "user", filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), []takeoutQuerySource{
		{WhereClause: takeoutWhereClauseForSites(sites), IncludeAnalytics: true, IncludeControl: true, UserID: &userID},
	})
}

// ExportSiteData exports active data to the specified format.
// Format is validated by the handler (xlsx, csv, parquet, json, ndjson).
func (s *TakeoutService) ExportSiteData(ctx context.Context, siteID uuid.UUID, format string) (string, error) {
	if err := os.MkdirAll(s.path, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	normalizedFormat := exportfmt.Normalize(format, exportfmt.FormatXLSX)
	filename := filepath.Join(s.path, fmt.Sprintf("site_takeout_%s_%d.%s", siteID, time.Now().Unix(), normalizedFormat))
	whereClause := fmt.Sprintf("site_id = '%s'", siteID)

	store := s.store
	if s.tenantStores != nil {
		analyticsStore, _, err := s.tenantStores.ResolveSiteStore(ctx, siteID)
		if err != nil {
			return "", fmt.Errorf("failed to resolve site analytics store: %w", err)
		}
		store = analyticsStore
		if analyticsStore != s.store {
			return s.exportTakeoutFromSources(ctx, "site", filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), []takeoutStoreSource{
				{Store: analyticsStore, Source: takeoutQuerySource{WhereClause: whereClause, IncludeAnalytics: true}},
				{Store: s.store, Source: takeoutQuerySource{WhereClause: whereClause, IncludeControl: true}},
			})
		}
	}

	return s.exportTakeoutFromStore(ctx, store, "site", filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), []takeoutQuerySource{
		{WhereClause: whereClause, IncludeAnalytics: true, IncludeControl: true},
	})
}

func (s *TakeoutService) ExportQRCodeData(ctx context.Context, siteID, qrCodeID uuid.UUID, format string) (string, error) {
	if err := os.MkdirAll(s.path, 0755); err != nil {
		return "", fmt.Errorf("failed to create export directory: %w", err)
	}

	normalizedFormat := exportfmt.Normalize(format, exportfmt.FormatXLSX)
	filename := filepath.Join(s.path, fmt.Sprintf("qr_takeout_%s_%d.%s", qrCodeID, time.Now().Unix(), normalizedFormat))

	analyticsStore := s.store
	if s.tenantStores != nil {
		resolved, _, err := s.tenantStores.ResolveSiteStore(ctx, siteID)
		if err != nil {
			return "", fmt.Errorf("failed to resolve qr analytics store: %w", err)
		}
		analyticsStore = resolved
	}

	if analyticsStore == s.store {
		return s.exportQRCodeTakeoutFromStore(ctx, s.store, filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), siteID, qrCodeID, true, true)
	}

	tempFiles := []string{
		filepath.Join(s.path, fmt.Sprintf("qr_takeout_control_%d.parquet", time.Now().UnixNano())),
		filepath.Join(s.path, fmt.Sprintf("qr_takeout_analytics_%d.parquet", time.Now().UnixNano())),
	}
	defer func() {
		for _, tempFile := range tempFiles {
			_ = os.Remove(tempFile)
		}
	}()

	if _, err := s.exportQRCodeTakeoutFromStore(ctx, s.store, tempFiles[0], exportfmt.FormatParquet, exportfmt.DuckDBCopyOptions(exportfmt.FormatParquet), siteID, qrCodeID, true, false); err != nil {
		return "", err
	}
	if _, err := s.exportQRCodeTakeoutFromStore(ctx, analyticsStore, tempFiles[1], exportfmt.FormatParquet, exportfmt.DuckDBCopyOptions(exportfmt.FormatParquet), siteID, qrCodeID, false, true); err != nil {
		return "", err
	}

	query := buildTakeoutMergeQuery(tempFiles, filename, exportfmt.DuckDBCopyOptions(normalizedFormat))
	err := s.store.WithDuckDBSession(ctx, database.DuckDBSessionOptions{
		Excel: normalizedFormat == exportfmt.FormatXLSX,
	}, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, query)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("failed to export qr data: %w", err)
	}
	return filename, nil
}

type takeoutQuerySource struct {
	WhereClause      string
	IncludeAnalytics bool
	IncludeControl   bool
	UserID           *uuid.UUID
}

type takeoutStoreSource struct {
	Store  *database.Store
	Source takeoutQuerySource
}

func (s *TakeoutService) exportTakeoutFromSources(ctx context.Context, label, filename, normalizedFormat, duckFormat string, sources []takeoutStoreSource) (string, error) {
	if len(sources) == 0 {
		return s.exportTakeoutFromStore(ctx, s.store, label, filename, normalizedFormat, duckFormat, []takeoutQuerySource{{WhereClause: "FALSE"}})
	}
	if len(sources) == 1 {
		return s.exportTakeoutFromStore(ctx, sources[0].Store, label, filename, normalizedFormat, duckFormat, []takeoutQuerySource{sources[0].Source})
	}

	tempFiles := make([]string, 0, len(sources))
	defer func() {
		for _, tempFile := range tempFiles {
			_ = os.Remove(tempFile)
		}
	}()

	for i, source := range sources {
		tempFile := filepath.Join(s.path, fmt.Sprintf("takeout_merge_%d_%d.parquet", time.Now().UnixNano(), i))
		tempFiles = append(tempFiles, tempFile)
		if _, err := s.exportTakeoutFromStore(ctx, source.Store, label, tempFile, exportfmt.FormatParquet, exportfmt.DuckDBCopyOptions(exportfmt.FormatParquet), []takeoutQuerySource{source.Source}); err != nil {
			return "", err
		}
	}
	if len(tempFiles) == 0 {
		return s.exportTakeoutFromStore(ctx, s.store, label, filename, normalizedFormat, duckFormat, []takeoutQuerySource{{WhereClause: "FALSE"}})
	}

	query := buildTakeoutMergeQuery(tempFiles, filename, duckFormat)
	err := s.store.WithDuckDBSession(ctx, database.DuckDBSessionOptions{
		Excel: normalizedFormat == exportfmt.FormatXLSX,
	}, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to export %s data: %w", label, err)
	}

	return filename, nil
}

func (s *TakeoutService) exportTakeoutFromStore(ctx context.Context, store *database.Store, label, filename, normalizedFormat, duckFormat string, sources []takeoutQuerySource) (string, error) {
	query := buildTakeoutQuery(sources, filename, duckFormat)
	err := store.WithDuckDBSession(ctx, database.DuckDBSessionOptions{
		Excel: normalizedFormat == exportfmt.FormatXLSX,
	}, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to export %s data: %w", label, err)
	}

	return filename, nil
}

func (s *TakeoutService) takeoutSourcesForSites(ctx context.Context, userID uuid.UUID, sites []api.Site) ([]takeoutStoreSource, error) {
	if len(sites) == 0 {
		return []takeoutStoreSource{{Store: s.store, Source: takeoutQuerySource{WhereClause: "FALSE", IncludeAnalytics: true, IncludeControl: true, UserID: &userID}}}, nil
	}

	sharedIDs := make([]uuid.UUID, 0)
	tenantIDsByStore := make(map[*database.Store][]uuid.UUID)
	for _, site := range sites {
		analyticsStore, _, err := s.tenantStores.ResolveSiteStore(ctx, site.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve analytics store for site %s: %w", site.ID, err)
		}
		if analyticsStore == s.store {
			sharedIDs = append(sharedIDs, site.ID)
			continue
		}
		tenantIDsByStore[analyticsStore] = append(tenantIDsByStore[analyticsStore], site.ID)
	}

	sources := make([]takeoutStoreSource, 0, len(tenantIDsByStore)+1)
	if len(sharedIDs) > 0 {
		sources = append(sources, takeoutStoreSource{
			Store:  s.store,
			Source: takeoutQuerySource{WhereClause: takeoutWhereClauseForSiteIDs(sharedIDs), IncludeAnalytics: true},
		})
	}
	sources = append(sources, takeoutStoreSource{
		Store:  s.store,
		Source: takeoutQuerySource{WhereClause: takeoutWhereClauseForSites(sites), IncludeControl: true, UserID: &userID},
	})

	for store, ids := range tenantIDsByStore {
		sources = append(sources, takeoutStoreSource{
			Store:  store,
			Source: takeoutQuerySource{WhereClause: takeoutWhereClauseForSiteIDs(ids), IncludeAnalytics: true},
		})
	}
	if len(sources) == 0 {
		return []takeoutStoreSource{{Store: s.store, Source: takeoutQuerySource{WhereClause: "FALSE", IncludeAnalytics: true, IncludeControl: true, UserID: &userID}}}, nil
	}
	return sources, nil
}

func (s *TakeoutService) CleanupExportFile(filename string) {
	if filename == "" {
		return
	}

	cleanedFile, ok := s.cleanExportPath(filename)
	if !ok {
		return
	}

	//nolint:gosec // cleaned path is constrained to a takeout export under the configured export directory.
	_ = os.Remove(cleanedFile)
}

func (s *TakeoutService) OpenExportFile(filename string) (*ExportFile, error) {
	cleanedFile, ok := s.cleanExportPath(filename)
	if !ok {
		return nil, fmt.Errorf("invalid takeout export path")
	}

	file, err := os.Open(cleanedFile) //nolint:gosec // cleaned path is constrained to a takeout export under the configured export directory.
	if err != nil {
		return nil, fmt.Errorf("open takeout export: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("stat takeout export: %w", err)
	}

	return &ExportFile{
		File: file,
		Info: info,
		Name: filepath.Base(cleanedFile),
	}, nil
}

func (s *TakeoutService) cleanExportPath(filename string) (string, bool) {
	cleanedFile := filepath.Clean(filename)
	base := filepath.Base(cleanedFile)
	if !strings.HasPrefix(base, "user_takeout_") && !strings.HasPrefix(base, "site_takeout_") && !strings.HasPrefix(base, "qr_takeout_") {
		return "", false
	}

	exportDir := filepath.Clean(s.path)
	rel, err := filepath.Rel(exportDir, cleanedFile)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return "", false
	}

	return cleanedFile, true
}

func buildTakeoutQuery(sources []takeoutQuerySource, filename, format string) string {
	if len(sources) == 0 {
		sources = []takeoutQuerySource{{WhereClause: "FALSE"}}
	}

	selects := make([]string, 0, len(sources)*12)
	for _, source := range sources {
		whereClause := source.WhereClause
		if whereClause == "" {
			whereClause = "FALSE"
		}
		includeAnalytics := source.IncludeAnalytics
		includeControl := source.IncludeControl
		if !includeAnalytics && !includeControl {
			includeAnalytics = true
			includeControl = true
		}
		if includeAnalytics {
			selects = append(selects,
				fmt.Sprintf("SELECT 'hit' as record_type, * FROM hits WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'event' as record_type, * FROM events WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'web_vital' as record_type, * FROM web_vitals WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'ai_fetch' as record_type, * FROM ai_fetches WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'qr_code_open' as record_type, * FROM qr_code_opens WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'goal' as record_type, * FROM goals WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'funnel' as record_type, * FROM funnels WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'imported_traffic' as record_type, * FROM imported_traffic_daily WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'imported_dimension' as record_type, * FROM imported_dimension_daily WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'imported_event' as record_type, * FROM imported_event_daily WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'imported_event_dimension' as record_type, * FROM imported_event_dimensions_daily WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'imported_event_property' as record_type, * FROM imported_event_properties_daily WHERE %s", whereClause),
			)
		}
		if includeControl {
			selects = append(selects,
				fmt.Sprintf("SELECT 'qr_code' AS record_type, * FROM qr_codes WHERE %s", whereClause),
				qrAssetTakeoutSelect(whereClause),
				fmt.Sprintf("SELECT 'qr_code_share_link' AS record_type, * FROM qr_code_share_links WHERE %s", whereClause),
				opportunityTakeoutSelect(whereClause),
				aiRunTakeoutSelect(whereClause),
				webhookTakeoutSelect(whereClause),
				webhookSubscriptionTakeoutSelect(whereClause),
				webhookDeliveryTakeoutSelect(whereClause),
				webhookDeliveryAttemptTakeoutSelect(whereClause),
			)
			if source.UserID != nil && *source.UserID != uuid.Nil {
				selects = append(selects, socialIdentityTakeoutSelect(*source.UserID))
			}
		}
	}
	if len(selects) == 0 {
		selects = append(selects, "SELECT 'empty' as record_type WHERE FALSE")
	}

	return fmt.Sprintf(`
	COPY (
		%s
	) TO '%s' (FORMAT %s);
`, strings.Join(selects, "\n\t\tUNION BY NAME\n\t\t"), escapeTakeoutSQLString(filename), format)
}

func socialIdentityTakeoutSelect(userID uuid.UUID) string {
	return fmt.Sprintf(`
		SELECT
			'social_identity' AS record_type,
			user_id,
			provider,
			subject,
			observed_email,
			linked_at,
			updated_at,
			last_used_at
		FROM social_identities
		WHERE user_id = '%s'
	`, userID)
}

func (s *TakeoutService) exportQRCodeTakeoutFromStore(ctx context.Context, store *database.Store, filename, normalizedFormat, duckFormat string, siteID, qrCodeID uuid.UUID, includeControl, includeAnalytics bool) (string, error) {
	query := buildQRCodeTakeoutQuery(filename, duckFormat, siteID, qrCodeID, includeControl, includeAnalytics)
	err := store.WithDuckDBSession(ctx, database.DuckDBSessionOptions{
		Excel: normalizedFormat == exportfmt.FormatXLSX,
	}, func(conn *sql.Conn) error {
		_, err := conn.ExecContext(ctx, query)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("failed to export qr data: %w", err)
	}
	return filename, nil
}

func buildQRCodeTakeoutQuery(filename, format string, siteID, qrCodeID uuid.UUID, includeControl, includeAnalytics bool) string {
	site := escapeTakeoutSQLString(siteID.String())
	qr := escapeTakeoutSQLString(qrCodeID.String())
	selects := []string{}
	if includeControl {
		selects = append(selects,
			fmt.Sprintf("SELECT 'qr_code' AS record_type, * FROM qr_codes WHERE site_id = '%s' AND id = '%s'", site, qr),
			qrAssetTakeoutSelect(fmt.Sprintf("site_id = '%s' AND qr_code_id = '%s'", site, qr)),
			fmt.Sprintf("SELECT 'qr_code_share_link' AS record_type, * FROM qr_code_share_links WHERE site_id = '%s' AND qr_code_id = '%s'", site, qr),
		)
	}
	if includeAnalytics {
		sessionScope := fmt.Sprintf("SELECT DISTINCT session_id FROM hits WHERE site_id = '%s' AND qr_code_id = '%s'", site, qr)
		selects = append(selects,
			fmt.Sprintf("SELECT 'hit' AS record_type, * FROM hits WHERE site_id = '%s' AND qr_code_id = '%s'", site, qr),
			fmt.Sprintf("SELECT 'qr_code_open' AS record_type, * FROM qr_code_opens WHERE site_id = '%s' AND qr_code_id = '%s'", site, qr),
			fmt.Sprintf("SELECT 'event' AS record_type, * FROM events WHERE site_id = '%s' AND session_id IN (%s)", site, sessionScope),
			fmt.Sprintf("SELECT 'web_vital' AS record_type, * FROM web_vitals WHERE site_id = '%s' AND session_id IN (%s)", site, sessionScope),
		)
	}
	if len(selects) == 0 {
		selects = append(selects, "SELECT 'empty' AS record_type WHERE FALSE")
	}
	return fmt.Sprintf(`
	COPY (
		%s
	) TO '%s' (FORMAT %s);
`, strings.Join(selects, "\n\t\tUNION BY NAME\n\t\t"), escapeTakeoutSQLString(filename), format)
}

func opportunityTakeoutSelect(whereClause string) string {
	return fmt.Sprintf(`
		SELECT
			'opportunity' AS record_type,
			id,
			team_id,
			site_id,
			kind,
			type_key,
			title_key,
			summary_key,
			action_key,
			digest_key,
			copy_params_json,
			impact_value,
			impact_label_key,
			confidence,
			score,
			score_breakdown_json,
			status,
			route_label_key,
			route_params_json,
			route_icon,
			detector_version,
			evidence_json,
			cited_evidence_ids_json,
			ai_run_id,
			generated_at,
			created_at,
			updated_at
		FROM opportunities
		WHERE %s
	`, whereClause)
}

func qrAssetTakeoutSelect(whereClause string) string {
	return fmt.Sprintf(`
		SELECT
			'qr_code_asset' AS record_type,
			qr_code_id,
			site_id,
			filename,
			content_type,
			byte_size,
			width,
			height,
			checksum,
			created_at,
			updated_at
		FROM qr_code_assets
		WHERE %s
	`, whereClause)
}

func aiRunTakeoutSelect(whereClause string) string {
	return fmt.Sprintf(`
		SELECT
			'ai_run' AS record_type,
			id,
			team_id,
			site_id,
			actor_id,
			actor_type,
			feature,
			provider,
			model,
			template_version,
			evidence_ids_json,
			input_hash,
			output_hash,
			input_tokens,
			output_tokens,
			total_tokens,
			tool_call_count,
			lifecycle_events_json,
			status,
			error_category,
			latency_ms,
			created_at
		FROM ai_runs
		WHERE %s
	`, whereClause)
}

func webhookTakeoutSelect(whereClause string) string {
	return fmt.Sprintf(`
		SELECT
			'webhook' AS record_type,
			id,
			site_id,
			name,
			description,
			enabled,
			created_at,
			updated_at
		FROM webhooks
		WHERE %s
	`, whereClause)
}

func webhookSubscriptionTakeoutSelect(whereClause string) string {
	return fmt.Sprintf(`
		SELECT
			'webhook_event_subscription' AS record_type,
			webhook_id,
			event_type
		FROM webhook_event_subscriptions
		WHERE webhook_id IN (SELECT id FROM webhooks WHERE %s)
	`, whereClause)
}

func webhookDeliveryTakeoutSelect(whereClause string) string {
	return fmt.Sprintf(`
		SELECT
			'webhook_delivery' AS record_type,
			id,
			event_id,
			webhook_id,
			site_id,
			event_type,
			webhook_name,
			status,
			attempt_count,
			next_attempt_at,
			last_attempt_at,
			completed_at,
			response_status,
			last_error_code,
			created_at,
			updated_at
		FROM webhook_deliveries
		WHERE webhook_id IN (SELECT id FROM webhooks WHERE %s)
	`, whereClause)
}

func webhookDeliveryAttemptTakeoutSelect(whereClause string) string {
	return fmt.Sprintf(`
		SELECT
			'webhook_delivery_attempt' AS record_type,
			id,
			delivery_id,
			site_id,
			attempt_number,
			status,
			response_status,
			error_code,
			started_at,
			completed_at,
			next_attempt_at
		FROM webhook_delivery_attempts
		WHERE delivery_id IN (
			SELECT id FROM webhook_deliveries
			WHERE webhook_id IN (SELECT id FROM webhooks WHERE %s)
		)
	`, whereClause)
}

func buildTakeoutMergeQuery(filenames []string, filename, format string) string {
	escapedFiles := make([]string, 0, len(filenames))
	for _, sourceFile := range filenames {
		escapedFiles = append(escapedFiles, fmt.Sprintf("'%s'", escapeTakeoutSQLString(sourceFile)))
	}
	return fmt.Sprintf(`
	COPY (
		SELECT * FROM read_parquet([%s], union_by_name = true)
	) TO '%s' (FORMAT %s);
`, strings.Join(escapedFiles, ", "), escapeTakeoutSQLString(filename), format)
}

func takeoutWhereClauseForSites(sites []api.Site) string {
	ids := make([]uuid.UUID, 0, len(sites))
	for _, site := range sites {
		ids = append(ids, site.ID)
	}
	return takeoutWhereClauseForSiteIDs(ids)
}

func takeoutWhereClauseForSiteIDs(siteIDs []uuid.UUID) string {
	if len(siteIDs) == 0 {
		return "FALSE"
	}

	ids := make([]string, 0, len(siteIDs))
	for _, siteID := range siteIDs {
		ids = append(ids, fmt.Sprintf("'%s'", siteID))
	}
	return fmt.Sprintf("site_id IN (%s)", strings.Join(ids, ", "))
}

func escapeTakeoutSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
