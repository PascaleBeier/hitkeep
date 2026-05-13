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
		sources, err := s.takeoutSourcesForSites(ctx, sites)
		if err != nil {
			return "", err
		}
		return s.exportTakeoutFromSources(ctx, "user", filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), sources)
	}

	return s.exportTakeoutFromStore(ctx, s.store, "user", filename, normalizedFormat, exportfmt.DuckDBCopyOptions(normalizedFormat), []takeoutQuerySource{
		{WhereClause: takeoutWhereClauseForSites(sites), IncludeAnalytics: true, IncludeControl: true},
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

type takeoutQuerySource struct {
	WhereClause      string
	IncludeAnalytics bool
	IncludeControl   bool
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

func (s *TakeoutService) takeoutSourcesForSites(ctx context.Context, sites []api.Site) ([]takeoutStoreSource, error) {
	if len(sites) == 0 {
		return []takeoutStoreSource{{Store: s.store, Source: takeoutQuerySource{WhereClause: "FALSE", IncludeAnalytics: true, IncludeControl: true}}}, nil
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
		Source: takeoutQuerySource{WhereClause: takeoutWhereClauseForSites(sites), IncludeControl: true},
	})

	for store, ids := range tenantIDsByStore {
		sources = append(sources, takeoutStoreSource{
			Store:  store,
			Source: takeoutQuerySource{WhereClause: takeoutWhereClauseForSiteIDs(ids), IncludeAnalytics: true},
		})
	}
	if len(sources) == 0 {
		return []takeoutStoreSource{{Store: s.store, Source: takeoutQuerySource{WhereClause: "FALSE", IncludeAnalytics: true, IncludeControl: true}}}, nil
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
	if !strings.HasPrefix(base, "user_takeout_") && !strings.HasPrefix(base, "site_takeout_") {
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

	selects := make([]string, 0, len(sources)*7)
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
				fmt.Sprintf("SELECT 'ai_fetch' as record_type, * FROM ai_fetches WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'goal' as record_type, * FROM goals WHERE %s", whereClause),
				fmt.Sprintf("SELECT 'funnel' as record_type, * FROM funnels WHERE %s", whereClause),
			)
		}
		if includeControl {
			selects = append(selects,
				opportunityTakeoutSelect(whereClause),
				aiRunTakeoutSelect(whereClause),
			)
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
