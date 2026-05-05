package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/nsqio/go-nsq"
	"golang.org/x/time/rate"

	"hitkeep/internal/blocking"
	"hitkeep/internal/cluster"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailer"
	"hitkeep/internal/mcpserver"
	"hitkeep/internal/searchconsole"
	"hitkeep/internal/server/admin"
	"hitkeep/internal/server/aifetch"
	serverauth "hitkeep/internal/server/auth"
	cloudhandlers "hitkeep/internal/server/cloud"
	"hitkeep/internal/server/events"
	"hitkeep/internal/server/goals"
	importhandlers "hitkeep/internal/server/imports"
	"hitkeep/internal/server/ingest"
	"hitkeep/internal/server/permissions"
	searchconsolereports "hitkeep/internal/server/searchconsolereports"
	sharehandlers "hitkeep/internal/server/share"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/server/sites"
	"hitkeep/internal/server/system"
	takeouthandlers "hitkeep/internal/server/takeout"
	"hitkeep/internal/server/user"
	"hitkeep/internal/takeout"
)

const (
	cacheControlImmutable = "public, max-age=31536000, immutable"
	cacheControlNoCache   = "no-cache, no-store, must-revalidate"
)

type Server struct {
	httpServer     *http.Server
	store          *database.Store
	cluster        *cluster.Manager
	producer       *nsq.Producer
	mailer         *mailer.Mailer
	conf           *config.Config
	ingestLimiter  *shared.IPRateLimiter
	apiLimiter     *shared.IPRateLimiter
	authLimiter    *shared.IPRateLimiter
	webhookLimiter *shared.IPRateLimiter
	ipFilter       *blocking.IPFilter
	ipFilterStop   context.CancelFunc
	spamFilter     *blocking.SpamFilter
	spamFilterStop context.CancelFunc
	takeout        *takeout.TakeoutService
	ctx            *shared.Context

	indexHTML   []byte
	scalarIndex []byte
}

func New(conf *config.Config, publicFS fs.FS, store *database.Store, tenantStores *database.TenantStoreManager, ent entitlements.Provider, cluster *cluster.Manager, producer *nsq.Producer, mailService *mailer.Mailer) *Server {
	config.NormalizeMCPConfig(conf)

	ingestLim := shared.NewIPRateLimiter(rate.Limit(conf.IngestRateLimit), conf.IngestBurst)
	apiLim := shared.NewIPRateLimiter(rate.Limit(conf.ApiRateLimit), conf.ApiBurst)
	authLim := shared.NewIPRateLimiter(rate.Limit(conf.AuthRateLimit), conf.AuthBurst)
	webhookLim := shared.NewIPRateLimiter(rate.Limit(conf.WebhookRateLimit), conf.WebhookBurst)
	var authState *shared.AuthStateStore
	if store != nil {
		authState = shared.NewAuthStateStore()
	}

	takeoutService := takeout.NewTakeoutService(store, "archive/takeout")

	var ipFilter *blocking.IPFilter
	var ipFilterStop context.CancelFunc
	if store != nil {
		ipFilter = blocking.NewIPFilter(store)
		filterCtx, cancel := context.WithCancel(context.Background())
		ipFilter.StartRefreshLoop(filterCtx)
		ipFilterStop = cancel
	}

	spamFilterPath := conf.SpamFilterPath
	if spamFilterPath == "" {
		spamFilterPath = conf.DataPath + "/spam-filter.json"
	}

	spamFilter := blocking.NewSpamFilter(spamFilterPath)
	if err := spamFilter.RefreshFromDisk(); err != nil {
		slog.Warn("Failed to load cached spam filter data; embedded defaults will be used", "error", err, "path", spamFilterPath)
	}

	isLeader := func() bool {
		return cluster != nil && cluster.IsLeader()
	}

	filterCtx, spamFilterCancel := context.WithCancel(context.Background())
	spamFilter.StartRefreshLoop(filterCtx, conf.SpamFilterAutoUpdate, time.Duration(conf.SpamFilterUpdateIntervalMin)*time.Minute, isLeader)
	spamFilterStop := spamFilterCancel

	s := &Server{
		store:          store,
		cluster:        cluster,
		producer:       producer,
		mailer:         mailService,
		conf:           conf,
		ingestLimiter:  ingestLim,
		apiLimiter:     apiLim,
		authLimiter:    authLim,
		webhookLimiter: webhookLim,
		ipFilter:       ipFilter,
		ipFilterStop:   ipFilterStop,
		spamFilter:     spamFilter,
		spamFilterStop: spamFilterStop,
		takeout:        takeoutService,
	}

	systemCounters := &database.SystemCounter{}
	backupStatus := &database.BackupStatusTracker{}
	backupStatus.SetConfig(
		conf.BackupPath != "",
		conf.BackupPath,
		conf.BackupIntervalMinutes,
		conf.BackupRetentionCount,
	)
	importStageCleanupStatus := &database.ImportStageCleanupStatusTracker{}
	importStageCleanupStatus.SetConfig(conf.ImportStageRetentionDays > 0, conf.ImportStageRetentionDays)
	mailTestTracker := &database.MailTestTracker{}

	s.ctx = &shared.Context{
		Store:          store,
		TenantStores:   tenantStores,
		Cluster:        cluster,
		Producer:       producer,
		Mailer:         mailService,
		Config:         conf,
		Takeout:        takeoutService,
		Entitlements:   ent,
		IngestLimiter:  ingestLim,
		ApiLimiter:     apiLim,
		AuthLimiter:    authLim,
		WebhookLimiter: webhookLim,
		AuthState:      authState,
		SearchConsole: searchconsole.NewGoogleClient(searchconsole.OAuthConfig{
			ClientID:     conf.GoogleSearchConsoleClientID,
			ClientSecret: conf.GoogleSearchConsoleClientSecret,
		}),
		IPFilter:                 ipFilter,
		SpamFilter:               spamFilter,
		StartedAt:                time.Now().UTC(),
		SystemCounters:           systemCounters,
		BackupStatus:             backupStatus,
		ImportStageCleanupStatus: importStageCleanupStatus,
		MailTestTracker:          mailTestTracker,
	}

	// Load static HTML into memory
	s.loadStaticAssets(publicFS)

	mux := http.NewServeMux()
	s.setupRoutes(mux, publicFS)

	s.httpServer = &http.Server{
		Addr:              conf.HTTPAddr,
		Handler:           shared.FetchMetadataMiddleware(conf.PublicURL, mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) ListenAndServe() error {
	slog.Info("HTTP server starting", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// BackupStatus returns the runtime backup tracker shared with background workers.
func (s *Server) BackupStatus() *database.BackupStatusTracker {
	if s == nil || s.ctx == nil {
		return nil
	}
	return s.ctx.BackupStatus
}

// ImportStageCleanupStatus returns the runtime tracker shared with import cleanup workers.
func (s *Server) ImportStageCleanupStatus() *database.ImportStageCleanupStatusTracker {
	if s == nil || s.ctx == nil {
		return nil
	}
	return s.ctx.ImportStageCleanupStatus
}

func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("HTTP server shutting down.")

	if s.ipFilterStop != nil {
		s.ipFilterStop()
	}
	if s.spamFilterStop != nil {
		s.spamFilterStop()
	}

	s.ingestLimiter.Stop()
	s.apiLimiter.Stop()
	s.authLimiter.Stop()
	s.webhookLimiter.Stop()

	return s.httpServer.Shutdown(ctx)
}

// loadStaticAssets reads index.html and scalar/index.html once at startup.
func (s *Server) loadStaticAssets(publicFS fs.FS) {
	// 1. Load Main SPA Index
	indexData, err := fs.ReadFile(publicFS, "index.html")
	if err != nil {
		slog.Warn("Frontend index.html not found. Dashboard will not be available.")
	} else {
		s.indexHTML = indexData
	}

	// 2. Load Scalar Index (API Docs)
	scalarData, err := fs.ReadFile(publicFS, "scalar/index.html")
	if err != nil {
		slog.Warn("Scalar index.html not found. API docs will not render.")
	} else {
		s.scalarIndex = scalarData
	}
}

func (s *Server) setupRoutes(mux *http.ServeMux, publicFS fs.FS) {
	ctx := s.ctx
	system.Register(mux, ctx)
	ingest.Register(mux, ctx)
	serverauth.Register(mux, ctx)
	cloudhandlers.Register(mux, ctx)
	user.Register(mux, ctx)
	permissions.Register(mux, ctx)
	admin.Register(mux, ctx)
	sites.Register(mux, ctx)
	goals.Register(mux, ctx)
	importhandlers.Register(mux, ctx)
	events.Register(mux, ctx)
	aifetch.Register(mux, ctx)
	searchconsolereports.Register(mux, ctx)
	takeouthandlers.Register(mux, ctx)
	sharehandlers.Register(mux, ctx)
	if s.conf.MCPEnabled && s.store != nil && (s.cluster == nil || s.cluster.IsLeader()) {
		slog.Info("MCP server route enabled", "path", s.conf.MCPPath)
		mcpserver.Register(mux, ctx, slog.Default())
	}

	// Static & SPA Handling
	mux.Handle("/", s.spaHandler(publicFS))
}

func (s *Server) spaHandler(publicFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(publicFS))

	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")

		if path == "scalar" || strings.HasPrefix(path, "scalar/") {
			if path == "scalar" || path == "scalar/" || path == "scalar/index.html" {
				s.serveScalarIndex(w)
				return
			}
			// Static scalar assets
			w.Header().Set("Cache-Control", cacheControlImmutable)
			fileServer.ServeHTTP(w, r)
			return
		}

		// 2. Handle Root (Explicit request for index)
		if path == "" || path == "index.html" {
			s.serveIndex(w)
			return
		}

		f, err := publicFS.Open(path)
		if err != nil {
			s.serveIndex(w)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err == nil && stat.IsDir() {
			s.serveIndex(w)
			return
		}

		// 4. Intelligent Caching for Static Assets
		// Angular/Webpack build artifacts contain hashes (e.g., main.7a2b9c.js).
		// We can tell browsers to cache these forever.
		if isImmutableAsset(path) {
			w.Header().Set("Cache-Control", cacheControlImmutable)
		} else {
			// Mutable assets (favicon.ico, manifest.json) must check ETag/Last-Modified
			w.Header().Set("Cache-Control", cacheControlNoCache)
		}

		fileServer.ServeHTTP(w, r)
	}
}

// serveIndex serves the Angular index.html from memory.
func (s *Server) serveIndex(w http.ResponseWriter) {
	if len(s.indexHTML) == 0 {
		http.Error(w, "Frontend index missing", http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", cacheControlNoCache)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.indexHTML)
}

func (s *Server) serveScalarIndex(w http.ResponseWriter) {
	if len(s.scalarIndex) == 0 {
		http.Error(w, "API docs missing", http.StatusNotFound)
		return
	}

	w.Header().Set("Cache-Control", cacheControlNoCache)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.scalarIndex)
}

// isImmutableAsset uses heuristics to determine if a file is an immutable build artifact.
func isImmutableAsset(path string) bool {
	if strings.HasPrefix(path, "flags/") && strings.HasSuffix(path, ".svg") {
		return true
	}
	if strings.HasPrefix(path, "browsers/") && strings.HasSuffix(path, ".avif") {
		return true
	}

	return strings.HasSuffix(path, ".js") ||
		strings.HasSuffix(path, ".css") ||
		strings.HasSuffix(path, ".woff2") ||
		strings.HasSuffix(path, ".woff") ||
		strings.HasSuffix(path, ".ttf")
}
