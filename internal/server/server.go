package server

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/nsqio/go-nsq"
	"golang.org/x/time/rate"

	"hitkeep/config"
	hitai "hitkeep/internal/ai"
	"hitkeep/internal/blocking"
	"hitkeep/internal/cluster"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/mailer"
	"hitkeep/internal/mcpserver"
	"hitkeep/internal/realtime"
	"hitkeep/internal/searchconsole"
	"hitkeep/internal/server/admin"
	"hitkeep/internal/server/aifetch"
	askaihandlers "hitkeep/internal/server/askai"
	serverauth "hitkeep/internal/server/auth"
	cloudhandlers "hitkeep/internal/server/cloud"
	"hitkeep/internal/server/events"
	"hitkeep/internal/server/goals"
	importhandlers "hitkeep/internal/server/imports"
	"hitkeep/internal/server/ingest"
	opportunityhandlers "hitkeep/internal/server/opportunities"
	"hitkeep/internal/server/permissions"
	qrcodehandlers "hitkeep/internal/server/qrcodes"
	searchconsolereports "hitkeep/internal/server/searchconsolereports"
	sharehandlers "hitkeep/internal/server/share"
	"hitkeep/internal/server/shared"
	"hitkeep/internal/server/sites"
	"hitkeep/internal/server/system"
	takeouthandlers "hitkeep/internal/server/takeout"
	"hitkeep/internal/server/user"
	webhookhandlers "hitkeep/internal/server/webhooks"
	"hitkeep/internal/socialauth"
	"hitkeep/internal/sso"
	"hitkeep/internal/takeout"
	"hitkeep/internal/webhookdispatcher"
)

const (
	cacheControlImmutable = "public, max-age=31536000, immutable"
	cacheControlNoCache   = "no-cache, no-store, must-revalidate"
)

type Server struct {
	httpServer       *http.Server
	logger           *slog.Logger
	store            *database.Store
	cluster          *cluster.Manager
	producer         *nsq.Producer
	mailer           *mailer.Mailer
	conf             *config.Config
	ingestLimiter    *shared.IPRateLimiter
	apiLimiter       *shared.IPRateLimiter
	authLimiter      *shared.IPRateLimiter
	webhookLimiter   *shared.IPRateLimiter
	ipFilter         *blocking.IPFilter
	ipFilterStop     context.CancelFunc
	spamFilter       *blocking.SpamFilter
	spamFilterStop   context.CancelFunc
	importRunnerStop func(context.Context) error
	takeout          *takeout.TakeoutService
	realtime         *realtime.Broker
	ctx              *shared.Context

	indexHTML   []byte
	scalarIndex []byte

	publicBasePath string
}

func New(conf *config.Config, publicFS fs.FS, store *database.Store, tenantStores *database.TenantStoreManager, ent entitlements.Provider, cluster *cluster.Manager, producer *nsq.Producer, mailService *mailer.Mailer, realtimeBroker *realtime.Broker, logger *slog.Logger) *Server {
	if logger == nil {
		panic("server: logger is required")
	}
	config.NormalizeMCPConfig(conf, logger)

	ingestLim := shared.NewIPRateLimiter(rate.Limit(conf.IngestRateLimit), conf.IngestBurst)
	apiLim := shared.NewIPRateLimiter(rate.Limit(conf.ApiRateLimit), conf.ApiBurst)
	authLim := shared.NewIPRateLimiter(rate.Limit(conf.AuthRateLimit), conf.AuthBurst)
	webhookLim := shared.NewIPRateLimiter(rate.Limit(conf.WebhookRateLimit), conf.WebhookBurst)
	var authState *shared.AuthStateStore
	if store != nil {
		authState = shared.NewAuthStateStore()
	}

	takeoutService := takeout.NewTakeoutServiceWithTenantStores(store, tenantStores, "archive/takeout")
	if realtimeBroker == nil {
		realtimeBroker = realtime.NewBroker()
	}

	var ipFilter *blocking.IPFilter
	var ipFilterStop context.CancelFunc
	if store != nil {
		ipFilter = blocking.NewIPFilter(store, logger)
		filterCtx, cancel := context.WithCancel(context.Background())
		ipFilter.StartRefreshLoop(filterCtx)
		ipFilterStop = cancel
	}

	spamFilterPath := conf.SpamFilterPath
	if spamFilterPath == "" {
		spamFilterPath = conf.DataPath + "/spam-filter.json"
	}

	spamFilter := blocking.NewSpamFilter(spamFilterPath, logger)
	if err := spamFilter.RefreshFromDisk(); err != nil {
		logger.Warn("Failed to load cached spam filter data; embedded defaults will be used", "error", err, "path", spamFilterPath)
	}

	isLeader := func() bool {
		return cluster != nil && cluster.IsLeader()
	}

	filterCtx, spamFilterCancel := context.WithCancel(context.Background())
	spamFilter.StartRefreshLoop(filterCtx, conf.SpamFilterAutoUpdate, time.Duration(conf.SpamFilterUpdateIntervalMin)*time.Minute, isLeader)
	spamFilterStop := spamFilterCancel

	s := &Server{
		logger:         logger,
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
		realtime:       realtimeBroker,
		publicBasePath: normalizePublicBasePath(conf.PublicURL),
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
	aiService, err := hitai.NewService(hitai.Config{
		Enabled:             conf.AIEnabled,
		Provider:            conf.AIProvider,
		Model:               conf.AIModel,
		BaseURL:             conf.AIBaseURL,
		Region:              conf.AIRegion,
		APIKey:              conf.AIAPIKey,
		Timeout:             time.Duration(conf.AITimeoutSeconds) * time.Second,
		RequestLimit:        conf.AIRequestLimit,
		TokenLimit:          conf.AITokenLimit,
		BudgetWindowMinutes: conf.AIBudgetWindowMinutes,
		ConfigMode:          aiConfigMode(conf),
	}, hitai.StoreRecorder{Store: store})
	if err != nil {
		logger.Warn("AI provider configuration rejected",
			"error_category", hitai.ClassifyError(err),
			slog.Group("ai",
				slog.Bool("enabled", conf.AIEnabled),
				slog.Bool("provider_configured", strings.TrimSpace(conf.AIProvider) != ""),
				slog.Bool("model_configured", strings.TrimSpace(conf.AIModel) != ""),
				slog.Bool("base_url_configured", strings.TrimSpace(conf.AIBaseURL) != ""),
				slog.Bool("api_key_configured", strings.TrimSpace(conf.AIAPIKey) != ""),
				slog.Bool("region_configured", strings.TrimSpace(conf.AIRegion) != ""),
			),
		)
		if aiService == nil {
			logger.Warn("AI service disabled because provider setup returned no service")
		}
	}

	var clusterState shared.ClusterState
	if cluster != nil {
		clusterState = cluster
	}

	s.ctx = &shared.Context{
		Logger:         logger,
		Store:          store,
		TenantStores:   tenantStores,
		Cluster:        clusterState,
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
		SSO:                      sso.NewRuntimeClient(conf.CloudHosted),
		AI:                       aiService,
		Realtime:                 realtimeBroker,
		IPFilter:                 ipFilter,
		SpamFilter:               spamFilter,
		Webhooks:                 webhookdispatcher.NewEmitter(store, producer, conf.Version, logger),
		StartedAt:                time.Now().UTC(),
		SystemCounters:           systemCounters,
		BackupStatus:             backupStatus,
		ImportStageCleanupStatus: importStageCleanupStatus,
		MailTestTracker:          mailTestTracker,
	}
	s.ctx.SocialAuth = socialauth.NewClient(s.ctx.SSO)

	// Load static HTML into memory
	s.loadStaticAssets(publicFS)

	mux := http.NewServeMux()
	s.setupRoutes(mux, publicFS)
	rootHandler := s.databaseAvailabilityMiddleware(newCrossOriginProtection().Handler(mux))
	handler := rootHandler
	if s.publicBasePath != "/" {
		handler = s.stripPublicBasePath(rootHandler)
	}
	handler = s.customTrackingHostMiddleware(publicFS, rootHandler, handler)
	handler = s.requestLoggingMiddleware(handler)

	s.httpServer = &http.Server{
		Addr:              conf.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) ListenAndServe() error {
	s.logger.Info("HTTP server starting", "addr", s.httpServer.Addr)
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
	s.logger.Info("HTTP server shutting down.")
	if s.ctx.Realtime != nil {
		s.ctx.Realtime.Close()
	}

	if s.importRunnerStop != nil {
		if err := s.importRunnerStop(ctx); err != nil {
			s.logger.Warn("Import runner did not stop cleanly", "error", err)
		}
	}

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
		s.logger.Warn("Frontend index.html not found. Dashboard will not be available.")
	} else {
		s.indexHTML = injectDashboardBasePath(indexData, s.publicBasePath)
	}

	// 2. Load Scalar Index (API Docs)
	scalarData, err := fs.ReadFile(publicFS, "scalar/index.html")
	if err != nil {
		s.logger.Warn("Scalar index.html not found. API docs will not render.")
	} else {
		s.scalarIndex = scalarData
	}
}

func normalizePublicBasePath(publicURL string) string {
	raw := strings.TrimSpace(publicURL)
	if raw == "" {
		return "/"
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "/"
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return "/"
	}

	cleaned := path.Clean("/" + parsed.EscapedPath())
	if cleaned == "." || cleaned == "/" {
		return "/"
	}
	return cleaned + "/"
}

func injectDashboardBasePath(indexHTML []byte, basePath string) []byte {
	baseTag := []byte(`<base href="/" />`)
	nextTag := []byte(`<base href="` + basePath + `" />`)
	if bytes.Contains(indexHTML, baseTag) {
		return bytes.Replace(indexHTML, baseTag, nextTag, 1)
	}

	compactBaseTag := []byte(`<base href="/">`)
	compactNextTag := []byte(`<base href="` + basePath + `">`)
	return bytes.Replace(indexHTML, compactBaseTag, compactNextTag, 1)
}

func (s *Server) stripPublicBasePath(next http.Handler) http.Handler {
	basePath := strings.TrimSuffix(s.publicBasePath, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == basePath {
			r = cloneRequestWithPath(r, "/")
		} else if strings.HasPrefix(r.URL.Path, basePath+"/") {
			strippedPath := strings.TrimPrefix(r.URL.Path, basePath)
			if strippedPath == "" {
				strippedPath = "/"
			}
			r = cloneRequestWithPath(r, strippedPath)
		} else if r.URL.Path != "/healthz" && r.URL.Path != "/readyz" && !isCaddyTLSAskPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func cloneRequestWithPath(r *http.Request, nextPath string) *http.Request {
	clone := r.Clone(r.Context())
	nextURL := *r.URL
	nextURL.Path = nextPath
	nextURL.RawPath = ""
	clone.URL = &nextURL
	return clone
}

func (s *Server) setupRoutes(mux *http.ServeMux, publicFS fs.FS) {
	ctx := s.ctx
	system.Register(mux, ctx)
	mux.HandleFunc("GET /internal/caddy/on-demand-tls/{token}", s.handleCaddyOnDemandTLSAsk())
	ingest.Register(mux, ctx)
	serverauth.Register(mux, ctx)
	cloudhandlers.Register(mux, ctx)
	user.Register(mux, ctx)
	permissions.Register(mux, ctx)
	admin.Register(mux, ctx)
	sites.Register(mux, ctx)
	goals.Register(mux, ctx)
	webhookhandlers.Register(mux, ctx)
	s.importRunnerStop = importhandlers.Register(mux, ctx)
	events.Register(mux, ctx)
	askaihandlers.Register(mux, ctx)
	opportunityhandlers.Register(mux, ctx)
	aifetch.Register(mux, ctx)
	searchconsolereports.Register(mux, ctx)
	qrcodehandlers.Register(mux, ctx)
	takeouthandlers.Register(mux, ctx)
	sharehandlers.Register(mux, ctx)
	if s.conf.MCPEnabled && s.store != nil && (s.cluster == nil || s.cluster.IsLeader()) {
		s.logger.Info("MCP server route enabled", "path", s.conf.MCPPath)
		mcpserver.Register(mux, ctx, s.logger)
	}

	// Static & SPA Handling
	mux.Handle("/", s.spaHandler(publicFS))
}

func aiConfigMode(conf *config.Config) string {
	if conf.CloudHosted {
		return "cloud_managed"
	}
	return "self_hosted"
}

func (s *Server) spaHandler(publicFS fs.FS) http.HandlerFunc {
	fileServer := http.FileServer(http.FS(publicFS))

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
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
			s.serveSPAFallback(w, r)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err == nil && stat.IsDir() {
			s.serveSPAFallback(w, r)
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

// serveSPAFallback serves the Angular shell only for document navigations. Asset
// and API clients must receive a real 404 instead of an HTML document that the
// browser then tries to parse as JavaScript, CSS, or JSON.
func (s *Server) serveSPAFallback(w http.ResponseWriter, r *http.Request) {
	if (r.Method == http.MethodGet || r.Method == http.MethodHead) && !s.isNonDashboardPath(r.URL.Path) && acceptsSPADocument(r) {
		s.serveIndex(w)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) isNonDashboardPath(requestPath string) bool {
	requestPath = "/" + strings.TrimPrefix(requestPath, "/")
	prefixes := []string{"/api", "/ingest", "/internal", "/.well-known", "/q", "/healthz", "/readyz", "/scalar"}
	if s.conf != nil {
		mcpPath := strings.TrimRight(strings.TrimSpace(s.conf.MCPPath), "/")
		if mcpPath != "" {
			prefixes = append(prefixes, mcpPath)
		}
	}
	for _, prefix := range prefixes {
		if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
			return true
		}
	}
	return false
}

func acceptsSPADocument(r *http.Request) bool {
	if path.Ext(r.URL.Path) != "" {
		return false
	}
	accept := strings.TrimSpace(strings.ToLower(r.Header.Get("Accept")))
	documentFetch := strings.EqualFold(strings.TrimSpace(r.Header.Get("Sec-Fetch-Dest")), "document")
	if accept == "" {
		return documentFetch
	}

	var htmlQuality float64
	htmlSpecified := false
	var textWildcardQuality float64
	textWildcardSpecified := false
	var anyQuality float64
	anySpecified := false
	for mediaRange := range strings.SplitSeq(accept, ",") {
		parts := strings.Split(mediaRange, ";")
		mediaType := strings.TrimSpace(parts[0])
		quality := 1.0
		for _, parameter := range parts[1:] {
			name, value, ok := strings.Cut(strings.TrimSpace(parameter), "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(name), "q") {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				quality = 0
			} else {
				quality = parsed
			}
		}
		switch mediaType {
		case "text/html":
			htmlSpecified = true
			htmlQuality = quality
		case "text/*":
			textWildcardSpecified = true
			textWildcardQuality = quality
		case "*/*":
			anySpecified = true
			anyQuality = quality
		}
	}
	if htmlSpecified {
		return htmlQuality > 0
	}
	if textWildcardSpecified {
		return textWildcardQuality > 0
	}
	if anySpecified && anyQuality <= 0 {
		return false
	}
	if documentFetch {
		return true
	}
	return false
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
