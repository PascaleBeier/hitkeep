package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/auth"
	"hitkeep/internal/blocking"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/mailer"
	"hitkeep/internal/server/shared"
)

func setupSystemTestEnv(t *testing.T) (*handler, *database.Store, *database.TenantStoreManager, uuid.UUID, uuid.UUID, uuid.UUID) {
	t.Helper()

	basePath := t.TempDir()
	store := database.NewStore(filepath.Join(basePath, "shared.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect store: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ownerUserID, err := store.CreateUser(context.Background(), "owner@example.com", "hash")
	if err != nil {
		t.Fatalf("create owner user: %v", err)
	}
	adminUserID, err := store.CreateUser(context.Background(), "admin@example.com", "hash")
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	regularUserID, err := store.CreateUser(context.Background(), "user@example.com", "hash")
	if err != nil {
		t.Fatalf("create regular user: %v", err)
	}

	if err := store.UpdateInstanceRole(context.Background(), ownerUserID, auth.InstanceOwner, ownerUserID); err != nil {
		t.Fatalf("promote owner: %v", err)
	}
	if err := store.UpdateInstanceRole(context.Background(), adminUserID, auth.InstanceAdmin, ownerUserID); err != nil {
		t.Fatalf("promote admin: %v", err)
	}

	tenantStores := database.NewTenantStoreManager(store, basePath)
	t.Cleanup(func() { _ = tenantStores.Close() })

	systemCounters := &database.SystemCounter{}
	backupStatus := &database.BackupStatusTracker{}
	backupStatus.SetConfig(false, "", 0, 0)
	importStageCleanupStatus := &database.ImportStageCleanupStatusTracker{}
	importStageCleanupStatus.SetConfig(true, 7)
	mailTestTracker := &database.MailTestTracker{}

	ctx := &shared.Context{
		Store:                    store,
		TenantStores:             tenantStores,
		SystemCounters:           systemCounters,
		BackupStatus:             backupStatus,
		ImportStageCleanupStatus: importStageCleanupStatus,
		MailTestTracker:          mailTestTracker,
		Config: &config.Config{
			PublicURL:                "http://localhost:8080",
			JWTSecret:                "test-secret",
			DBPath:                   filepath.Join(basePath, "shared.db"),
			DataPath:                 basePath,
			ImportStageRetentionDays: 7,
		},
		StartedAt: time.Now().UTC(),
	}

	return &handler{ctx: ctx}, store, tenantStores, ownerUserID, adminUserID, regularUserID
}

func TestHandleGetSystem(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)
	h.ctx.Config.MCPEnabled = true
	h.ctx.Config.MCPPath = "/agent"
	h.ctx.Config.MCPDocsEnabled = true
	h.ctx.Config.MCPDocsURL = "https://docs.example.com"
	h.ctx.Config.BackupPath = "s3://hitkeep/backups"
	h.ctx.Config.SpamFilterAutoUpdate = true
	h.ctx.Config.SpamFilterUpdateIntervalMin = 60
	h.ctx.Config.MailDriver = "smtp"
	h.ctx.Config.GoogleSearchConsoleClientID = "gsc-client-id"
	h.ctx.Config.GoogleSearchConsoleClientSecret = "gsc-client-secret"
	h.ctx.Config.CloudHosted = true
	h.ctx.Config.CloudPlanName = "Pro"
	h.ctx.Config.CloudSignupEnabled = true
	h.ctx.Config.StripeSecretKey = "sk_test_123"
	h.ctx.Mailer = mailer.NewWithDriver(&adminTestMailDriver{}, h.ctx.Config)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetSystem().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var info api.SystemInfo
	if err := json.NewDecoder(w.Body).Decode(&info); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if info.PublicURL != "http://localhost:8080" {
		t.Fatalf("expected public_url 'http://localhost:8080', got %q", info.PublicURL)
	}
	if info.RuntimeMode != "cloud" {
		t.Fatalf("expected runtime_mode cloud, got %q", info.RuntimeMode)
	}
	if _, ok := info.ConfigFlags["user_count"]; ok {
		t.Fatal("did not expect user_count to be reported as a config flag")
	}
	if _, ok := info.ConfigFlags["site_count"]; ok {
		t.Fatal("did not expect site_count to be reported as a config flag")
	}

	features := featureStatusByKey(info.EnabledFeatures)
	for _, key := range []string{"mcp", "mcp_docs", "automatic_backups", "spam_auto_update", "mail_delivery", "google_search_console", "managed_cloud", "cloud_signup", "billing"} {
		feature, ok := features[key]
		if !ok {
			t.Fatalf("expected feature %q to be reported", key)
		}
		if !feature.Enabled {
			t.Fatalf("expected feature %q to be enabled", key)
		}
	}
	if features["automatic_backups"].Detail != "s3" {
		t.Fatalf("expected S3 backup detail, got %q", features["automatic_backups"].Detail)
	}
	if features["spam_auto_update"].Detail != "1h" {
		t.Fatalf("expected spam update interval detail 1h, got %q", features["spam_auto_update"].Detail)
	}
	if features["managed_cloud"].Detail != "Pro" {
		t.Fatalf("expected managed cloud detail Pro, got %q", features["managed_cloud"].Detail)
	}
	if features["google_search_console"].Detail != "oauth" {
		t.Fatalf("expected Search Console oauth detail, got %q", features["google_search_console"].Detail)
	}
}

func TestSystemRuntimeModeDefaultsToOSS(t *testing.T) {
	conf := &config.Config{}
	if mode := systemRuntimeMode(conf); mode != "oss" {
		t.Fatalf("expected oss runtime mode, got %q", mode)
	}
}

func featureStatusByKey(features []api.SystemFeatureStatus) map[string]api.SystemFeatureStatus {
	byKey := make(map[string]api.SystemFeatureStatus, len(features))
	for _, feature := range features {
		byKey[feature.Key] = feature
	}
	return byKey
}

func TestHandleGetHealth(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/health", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetHealth().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var health api.SystemHealth
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if health.Database != "ok" {
		t.Fatalf("expected database 'ok', got %q", health.Database)
	}
}

func TestHandleGetSearchConsoleReportsCredentialAndSyncHealth(t *testing.T) {
	h, store, _, ownerID, _, _ := setupSystemTestEnv(t)
	h.ctx.Config.GoogleSearchConsoleClientID = "client-id"
	h.ctx.Config.GoogleSearchConsoleClientSecret = "client-secret"
	seedSearchConsoleNeedsAttentionSystemStatus(t, store, ownerID)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/search-console", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetSearchConsole().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status api.SystemSearchConsoleStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.Status != "needs_attention" {
		t.Fatalf("expected needs_attention status, got %q", status.Status)
	}
	if status.CredentialsStatus != "configured" || status.WorkerStatus != "enabled" {
		t.Fatalf("expected configured credentials and enabled worker, got credentials=%q worker=%q", status.CredentialsStatus, status.WorkerStatus)
	}
	if status.ConnectedTeams != 1 || status.MappedSites != 1 || status.NeedsAttentionSyncs != 1 {
		t.Fatalf("unexpected Search Console counts: %+v", status)
	}
	if status.LastAttemptAt == nil {
		t.Fatalf("expected last_attempt_at")
	}
}

func seedSearchConsoleNeedsAttentionSystemStatus(t *testing.T, store *database.Store, ownerID uuid.UUID) {
	t.Helper()

	ctx := context.Background()
	team, err := store.CreateTenant(ctx, ownerID, "Search Console Team", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	site, err := store.CreateSite(ctx, ownerID, "gsc-status.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	now := time.Now().UTC()
	upsertSearchConsoleSystemFixtures(t, store, team.ID, site.ID, ownerID, now)
}

func upsertSearchConsoleSystemFixtures(t *testing.T, store *database.Store, teamID, siteID, ownerID uuid.UUID, now time.Time) {
	t.Helper()

	ctx := context.Background()
	if err := store.UpsertGoogleSearchConsoleConnection(ctx, database.GoogleSearchConsoleConnectionInput{
		TeamID:             teamID,
		ConnectedByUserID:  ownerID,
		GoogleAccountEmail: "owner@example.com",
		AccessToken:        "access-token",
		RefreshToken:       "refresh-token",
		ConnectedAt:        now,
	}); err != nil {
		t.Fatalf("upsert Search Console connection: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSiteMapping(ctx, database.GoogleSearchConsoleSiteMappingInput{
		SiteID:      siteID,
		TeamID:      teamID,
		PropertyURI: "sc-domain:gsc-status.example",
		MappedBy:    ownerID,
		MappedAt:    now,
	}); err != nil {
		t.Fatalf("upsert Search Console mapping: %v", err)
	}
	if err := store.UpsertGoogleSearchConsoleSyncState(ctx, database.GoogleSearchConsoleSyncStateInput{
		SiteID:            siteID,
		TeamID:            teamID,
		State:             "needs_attention",
		LastAttemptAt:     &now,
		LastErrorCategory: "authorization_revoked",
	}); err != nil {
		t.Fatalf("upsert Search Console sync state: %v", err)
	}
}

func TestHandleGetSearchConsoleReportsMissingCredentialsWithoutSecrets(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/search-console", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetSearchConsole().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status api.SystemSearchConsoleStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.Status != "not_configured" || status.CredentialsStatus != "missing" || status.WorkerStatus != "disabled" {
		t.Fatalf("expected not configured status, got %+v", status)
	}
	if strings.Contains(w.Body.String(), "client") || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), "token") {
		t.Fatalf("Search Console system status leaked sensitive credential wording: %s", w.Body.String())
	}
}

type testPinger struct {
	err error
}

func (p testPinger) Ping() error {
	return p.err
}

func TestWorkerHealthStatus(t *testing.T) {
	tests := []struct {
		name     string
		leader   bool
		producer nsqPinger
		want     string
		wantOK   bool
	}{
		{name: "follower is standby", leader: false, want: "standby", wantOK: true},
		{name: "leader missing producer", leader: true, want: "unavailable", wantOK: false},
		{name: "leader ping ok", leader: true, producer: testPinger{}, want: "ok", wantOK: true},
		{name: "leader ping error", leader: true, producer: testPinger{err: fmt.Errorf("nsq down")}, want: "error: nsq down", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := workerHealthStatus(tt.leader, tt.producer)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("workerHealthStatus() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestHandleGetStorage(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/storage", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetStorage().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var storage api.SystemStorage
	if err := json.NewDecoder(w.Body).Decode(&storage); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if storage.SharedDBPath == "" {
		t.Fatal("expected non-empty shared_db_path")
	}
	if storage.SharedDBBytes <= 0 {
		t.Fatalf("expected positive shared_db_bytes, got %d", storage.SharedDBBytes)
	}
	if _, _, err := filesystemUsage(h.ctx.Config.DataPath); err == nil {
		if storage.DiskTotal <= 0 {
			t.Fatalf("expected positive disk_total_bytes, got %d", storage.DiskTotal)
		}
		if storage.DiskAvailable <= 0 {
			t.Fatalf("expected positive disk_available_bytes, got %d", storage.DiskAvailable)
		}
	}
}

func TestHandleGetIngestStats(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/ingest", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetIngestStats().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats api.SystemIngestStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Should have zero values but not error
	if stats.RecentHits < 0 {
		t.Fatalf("unexpected negative hits: %d", stats.RecentHits)
	}
}

func TestHandleGetIngestStatsAggregatesTenantStores(t *testing.T) {
	h, store, tenantStores, ownerID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	team, err := store.CreateTenant(ctx, ownerID, "Tenant Ingest", "")
	if err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := store.SetActiveTenantID(ctx, ownerID, team.ID); err != nil {
		t.Fatalf("set active tenant: %v", err)
	}
	site, err := store.CreateSite(ctx, ownerID, "tenant-ingest.test")
	if err != nil {
		t.Fatalf("create tenant site: %v", err)
	}

	tenantStore, _, err := tenantStores.ResolveSiteStore(ctx, site.ID)
	if err != nil {
		t.Fatalf("resolve tenant store: %v", err)
	}
	now := time.Now().UTC()
	if err := tenantStore.CreateHit(ctx, &api.Hit{
		ID:        uuid.New(),
		SiteID:    site.ID,
		SessionID: uuid.New(),
		PageID:    uuid.New(),
		Timestamp: now,
		Path:      "/tenant",
	}); err != nil {
		t.Fatalf("create tenant hit: %v", err)
	}
	if err := tenantStore.CreateEvent(ctx, &api.Event{
		ID:         uuid.New(),
		SiteID:     site.ID,
		SessionID:  uuid.New(),
		Name:       "tenant.event",
		Properties: map[string]any{"scope": "tenant"},
		Timestamp:  now,
	}); err != nil {
		t.Fatalf("create tenant event: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/ingest", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetIngestStats().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var stats api.SystemIngestStats
	if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stats.RecentHits < 1 {
		t.Fatalf("expected tenant hit to be counted, got %d", stats.RecentHits)
	}
	if stats.RecentEvents < 1 {
		t.Fatalf("expected tenant event to be counted, got %d", stats.RecentEvents)
	}
}

func TestHandleGetBackups(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/backups", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetBackups().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status api.SystemBackupStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Backups disabled by default
	if status.Enabled {
		t.Fatal("expected backups disabled by default")
	}
}

func TestHandleGetSpamFilter(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)
	generatedAt := time.Date(2026, 4, 28, 12, 0, 0, 0, time.UTC)
	spamPath := filepath.Join(t.TempDir(), "spam-filter.json")
	if err := blocking.SaveSpamFeedData(spamPath, blocking.SpamFeedData{
		GeneratedAt:          generatedAt,
		ReferrerHostDenylist: []string{"spam.example"},
		NetworkDenylist:      []string{"203.0.113.0/24"},
	}); err != nil {
		t.Fatalf("save spam feed data: %v", err)
	}
	filter := blocking.NewSpamFilter(spamPath)
	if err := filter.RefreshFromDisk(); err != nil {
		t.Fatalf("refresh spam filter: %v", err)
	}
	h.ctx.SpamFilter = filter
	h.ctx.Config.SpamFilterPath = spamPath

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/spam-filter", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetSpamFilter().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status api.SystemSpamStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.DBPath == "" {
		t.Fatal("expected non-empty db_path")
	}
	if status.RuleCount != 2 {
		t.Fatalf("expected rule_count 2, got %d", status.RuleCount)
	}
	if status.LastRefresh == nil || !status.LastRefresh.Equal(generatedAt) {
		t.Fatalf("expected last_refresh %s, got %v", generatedAt.Format(time.RFC3339), status.LastRefresh)
	}
}

func TestHandleGetCaches(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/caches", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetCaches().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status api.SystemCacheStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.PermissionsCache.MaxSize != 8192 {
		t.Fatalf("expected max size 8192, got %d", status.PermissionsCache.MaxSize)
	}
}

func TestHandleGetMail(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/mail", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetMail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status api.SystemMailStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if status.Configured {
		t.Fatal("expected mail not configured in test")
	}
}

func TestHandleSpamRefreshAction(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	// Without spam filter, should return 503
	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/spam-filter/refresh", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleRefreshSpamFilter().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleGetImportStageCleanup(t *testing.T) {
	h, store, _, ownerID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "cleanup-status@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "cleanup-status.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	fileID := uuid.New()
	importJob, err := store.CreateSiteImportUpload(ctx, site.ID, userID, "plausible", []database.ImportFileCreate{
		{
			ID:           fileID,
			Filename:     "status.csv",
			RelativePath: filepath.Join("imports", site.ID.String(), fileID.String()+"-status.csv"),
			SizeBytes:    42,
		},
	})
	if err != nil {
		t.Fatalf("create import upload: %v", err)
	}
	old := time.Now().UTC().AddDate(0, 0, -8)
	if _, err := store.DB().ExecContext(ctx, `
		UPDATE site_imports
		SET status = ?, created_at = ?, updated_at = ?, validated_at = ?, finished_at = ?
		WHERE id = ?
	`, database.ImportStatusCompleted, old, old, old, old, importJob.ID); err != nil {
		t.Fatalf("age import: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/import-stage-cleanup", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetImportStageCleanup().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status api.SystemImportStageCleanupStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !status.Enabled || status.RetentionDays != 7 || status.StaleImports != 1 || status.StaleFiles != 1 || status.StaleBytes != 42 {
		t.Fatalf("unexpected cleanup status: %+v", status)
	}
}

func TestHandleRunImportStageCleanup(t *testing.T) {
	h, store, _, ownerID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	userID, err := store.CreateUser(ctx, "cleanup-run@example.com", "hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	site, err := store.CreateSite(ctx, userID, "cleanup-run.example")
	if err != nil {
		t.Fatalf("create site: %v", err)
	}
	fileID := uuid.New()
	relativePath := filepath.Join("imports", site.ID.String(), fileID.String()+"-run.csv")
	importJob, err := store.CreateSiteImportUpload(ctx, site.ID, userID, "plausible", []database.ImportFileCreate{
		{
			ID:           fileID,
			Filename:     "run.csv",
			RelativePath: relativePath,
			SizeBytes:    6,
		},
	})
	if err != nil {
		t.Fatalf("create import upload: %v", err)
	}
	path := filepath.Join(h.ctx.Config.DataPath, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("create stage dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("abcdef"), 0600); err != nil {
		t.Fatalf("write stage file: %v", err)
	}
	if err := store.UpdateImportFileProgress(ctx, importJob.ID, fileID, 6, ""); err != nil {
		t.Fatalf("mark uploaded: %v", err)
	}
	old := time.Now().UTC().AddDate(0, 0, -8)
	if _, err := store.DB().ExecContext(ctx, `
		UPDATE site_imports
		SET status = ?, created_at = ?, updated_at = ?, validated_at = ?, finished_at = ?
		WHERE id = ?
	`, database.ImportStatusValidated, old, old, old, old, importJob.ID); err != nil {
		t.Fatalf("age import: %v", err)
	}

	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/import-stage-cleanup/run", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleRunImportStageCleanup().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.SystemImportStageCleanupRunResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Result.FilesCleaned != 1 || resp.Result.ImportsMarkedFailed != 1 {
		t.Fatalf("unexpected run response: %+v", resp)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected staged file removed, stat err: %v", err)
	}

	entries, _, err := store.ListInstanceAuditEntries(ctx, database.InstanceAuditFilter{Action: "import_stage_cleanup.run", Limit: 10})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(entries) != 1 || entries[0].Outcome != "success" {
		t.Fatalf("expected success audit entry, got %+v", entries)
	}
}

func TestHandleMailTestAction(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	// Without mailer, should return 503
	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/mail/test", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleTestMail().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleListAudit(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/audit", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleListAudit().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.InstanceAuditListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Empty audit log is valid
	if resp.Total != 0 {
		t.Fatalf("expected empty audit, got %d entries", resp.Total)
	}
}

func TestHandleExportAuditJSON(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system", nil), ownerID)
	h.appendAudit(req, "export.test", "system", "", "", "success", "Export test")

	exportReq := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/audit/export", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleExportAudit().ServeHTTP(w, exportReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Fatalf("expected JSON content type, got %q", contentType)
	}

	var entries []api.InstanceAuditEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one exported entry")
	}
}

func TestHandleAuditRejectsInvalidFilters(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	tests := []struct {
		name   string
		path   string
		handle http.HandlerFunc
	}{
		{name: "list actor", path: "/api/admin/system/audit?actor_id=not-a-uuid", handle: h.handleListAudit()},
		{name: "list from", path: "/api/admin/system/audit?from=not-a-date", handle: h.handleListAudit()},
		{name: "list to", path: "/api/admin/system/audit?to=not-a-date", handle: h.handleListAudit()},
		{name: "list offset", path: "/api/admin/system/audit?offset=-1", handle: h.handleListAudit()},
		{name: "export actor", path: "/api/admin/system/audit/export?actor_id=not-a-uuid", handle: h.handleExportAudit()},
		{name: "export from", path: "/api/admin/system/audit/export?from=not-a-date", handle: h.handleExportAudit()},
		{name: "export limit", path: "/api/admin/system/audit/export?limit=-1", handle: h.handleExportAudit()},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := withAdminTestUser(httptest.NewRequest(http.MethodGet, tc.path, nil), ownerID)
			w := httptest.NewRecorder()
			tc.handle.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestHandleAuditAppendsAndListsEntries(t *testing.T) {
	h, store, _, ownerID, _, _ := setupSystemTestEnv(t)
	ctx := context.Background()

	// Append an audit entry via helper
	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system", nil), ownerID)
	h.appendAudit(req, "test.action", "test_type", "test-1", "Test Label", "success", "Test details")

	// List audit entries
	listReq := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/audit", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleListAudit().ServeHTTP(w, listReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.InstanceAuditListResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total < 1 {
		t.Fatalf("expected at least 1 audit entry, got %d", resp.Total)
	}

	found := false
	for _, entry := range resp.Entries {
		if entry.Action == "test.action" {
			found = true
			if entry.TargetType != "test_type" {
				t.Fatalf("expected target_type 'test_type', got %q", entry.TargetType)
			}
			if entry.Outcome != "success" {
				t.Fatalf("expected outcome 'success', got %q", entry.Outcome)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected test audit entry not found in list")
	}

	_ = ctx
	_ = store
}

func TestHandleExportAuditHonorsLimit(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system", nil), ownerID)
	h.appendAudit(req, "export.limit", "system", "1", "First", "success", "Export limit test")
	h.appendAudit(req, "export.limit", "system", "2", "Second", "success", "Export limit test")
	h.appendAudit(req, "export.limit", "system", "3", "Third", "success", "Export limit test")

	exportReq := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/audit/export?action=export.limit&limit=2", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleExportAudit().ServeHTTP(w, exportReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var entries []api.InstanceAuditEntry
	if err := json.NewDecoder(w.Body).Decode(&entries); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 exported entries, got %d", len(entries))
	}
}

func TestHandleExportAuditCSV(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system", nil), ownerID)
	h.appendAudit(req, "csv.test", "system", "", "", "success", "CSV export test")

	exportReq := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/audit/export?format=csv", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleExportAudit().ServeHTTP(w, exportReq)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/csv" {
		t.Fatalf("expected CSV content type, got %q", contentType)
	}

	body := w.Body.String()
	if !strings.Contains(body, "action") {
		t.Fatal("expected CSV header row with 'action'")
	}
	if !strings.Contains(body, "csv.test") {
		t.Fatal("expected CSV data row with 'csv.test'")
	}
}

func TestHandleMailTestSuccess(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	drv := &adminTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(drv, h.ctx.Config)

	h.ctx.Config.MailHost = "localhost"
	h.ctx.Config.MailPort = 25

	body := strings.NewReader(`{}`)
	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/mail/test", body), ownerID)
	w := httptest.NewRecorder()
	h.handleTestMail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if drv.subject == "" {
		t.Fatal("expected mail driver to have sent an email")
	}
	if !strings.Contains(drv.subject, "HitKeep System Test Email") {
		t.Fatalf("expected test email subject, got %q", drv.subject)
	}
	if len(drv.recipients) != 1 || drv.recipients[0] != "owner@example.com" {
		t.Fatalf("expected default recipient owner@example.com, got %#v", drv.recipients)
	}
}

func TestHandleMailTestUsesRequestedRecipient(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	drv := &adminTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(drv, h.ctx.Config)

	body := strings.NewReader(`{"email":"ops@example.com"}`)
	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/mail/test", body), ownerID)
	w := httptest.NewRecorder()
	h.handleTestMail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if len(drv.recipients) != 1 || drv.recipients[0] != "ops@example.com" {
		t.Fatalf("expected requested recipient ops@example.com, got %#v", drv.recipients)
	}
}

func TestHandleMailTestRejectsInvalidRecipient(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	drv := &adminTestMailDriver{}
	h.ctx.Mailer = mailer.NewWithDriver(drv, h.ctx.Config)

	body := strings.NewReader(`{"email":"not-an-email"}`)
	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/mail/test", body), ownerID)
	w := httptest.NewRecorder()
	h.handleTestMail().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}

	if len(drv.recipients) != 0 {
		t.Fatalf("expected no outbound email, got %#v", drv.recipients)
	}
}

// failMailDriver returns an error on Send to simulate a broken mail transport.
type failMailDriver struct{}

func (d *failMailDriver) Send(_ []string, _, _, _ string) error {
	return errors.New("simulated mail transport failure")
}
func (d *failMailDriver) Close() error { return nil }

func TestHandleMailTestFailure(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	h.ctx.Mailer = mailer.NewWithDriver(&failMailDriver{}, h.ctx.Config)

	body := strings.NewReader(`{}`)
	req := withAdminTestUser(httptest.NewRequest(http.MethodPost, "/api/admin/system/mail/test", body), ownerID)
	w := httptest.NewRecorder()
	h.handleTestMail().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSystemMailRedaction(t *testing.T) {
	h, _, _, ownerID, _, _ := setupSystemTestEnv(t)

	h.ctx.Config.MailUsername = "admin@example.com"
	h.ctx.Config.MailPassword = "super-secret-password"
	h.ctx.Config.MailHost = "smtp.example.com"
	h.ctx.Config.MailPort = 587

	req := withAdminTestUser(httptest.NewRequest(http.MethodGet, "/api/admin/system/mail", nil), ownerID)
	w := httptest.NewRecorder()
	h.handleGetMail().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var status api.SystemMailStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !status.Configured {
		t.Fatal("expected mail configured")
	}
	if status.Host != "smtp.example.com" {
		t.Fatalf("expected host 'smtp.example.com', got %q", status.Host)
	}
	if !strings.Contains(status.Username, "****") {
		t.Fatalf("expected username to be redacted, got %q", status.Username)
	}
	if !status.PasswordSet {
		t.Fatal("expected password_set true")
	}
	// Full password should not appear in response
	if strings.Contains(status.Username, "super-secret") {
		t.Fatal("username should not leak full value")
	}
}
