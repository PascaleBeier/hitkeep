package hitkeepcmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/nsqio/go-nsq"
	"github.com/nsqio/nsq/nsqd"
	"golang.org/x/sync/errgroup"

	"hitkeep/internal/cluster"
	"hitkeep/internal/config"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/hklog"
	"hitkeep/internal/ingest"
	"hitkeep/internal/mailer"
	"hitkeep/internal/realtime"
	"hitkeep/internal/searchconsole"
	"hitkeep/internal/server"
	"hitkeep/internal/webhookdispatcher"
	"hitkeep/internal/worker"
	"hitkeep/public"
)

var Version = "snapshot"

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func Run() {
	conf := config.Load()
	conf.Version = Version

	logLevel, err := hklog.ParseLevel(conf.LogLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid log level '%s', defaulting to INFO: %v\n", conf.LogLevel, err)
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	if conf.Healthcheck {
		if err := runHealthcheck(conf); err != nil {
			fmt.Fprintf(os.Stderr, "Healthcheck failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	defer func() {
		if r := recover(); r != nil {
			slog.Error("Application startup panicked", "error", r)
			os.Exit(1)
		}
	}()

	slog.Info("Starting HitKeep", "version", Version, "log_level", logLevel.String(), "config", conf)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, gCtx := errgroup.WithContext(ctx)

	clusterManager, err := cluster.NewManager(conf, logger)
	check(err)
	defer func() {
		if err := clusterManager.Shutdown(); err != nil {
			slog.Error("Failed to shutdown cluster manager", "error", err)
		}
	}()

	publicFS := public.FS()
	check(err)

	mailSvc, err := mailer.New(conf)
	if err != nil {
		slog.Warn("Mailer not configured, email features will not work", "error", err)
	}

	var store *controlstore.Store
	var tenantMgr *database.TenantStoreManager
	var producer *nsq.Producer
	ent := entitlements.NewProvider(conf)
	realtimeBroker := realtime.NewBroker()

	if clusterManager.IsLeader() {
		var leaderShutdown func()

		store, tenantMgr, producer, leaderShutdown, err = startLeaderServices(gCtx, conf, logger, logLevel, realtimeBroker)
		check(err)

		// Start Retention Worker
		var s3Conf *worker.S3Config
		if worker.IsS3ArchivePath(conf.ArchivePath) {
			s3Conf = &worker.S3Config{
				AccessKeyID:     conf.S3AccessKeyID,
				SecretAccessKey: conf.S3SecretAccessKey,
				SessionToken:    conf.S3SessionToken,
				Region:          conf.S3Region,
				Endpoint:        conf.S3Endpoint,
				URLStyle:        conf.S3URLStyle,
				UseSSL:          conf.S3UseSSL,
			}
			if s3Conf.AccessKeyID != "" {
				slog.Info("S3 archive enabled", "mode", "static credentials", "region", s3Conf.Region)
			} else {
				slog.Info("S3 archive enabled", "mode", "credential chain", "region", s3Conf.Region)
			}
		}
		retentionWorker := worker.NewRetentionWorker(tenantMgr, conf.ArchivePath, conf.DataRetentionDays, s3Conf, conf.DataPath)
		go retentionWorker.Start(gCtx)

		// Start Rollup Backfill Worker
		rollupWorker := worker.NewRollupBackfillWorker(tenantMgr)
		go rollupWorker.Start(gCtx)

		// Start Report Worker
		reportWorker := worker.NewReportWorker(tenantMgr, mailSvc, conf.PublicURL, conf.JWTSecret).
			WithEntitlements(entitlements.NewService(store, ent, conf))
		go reportWorker.Start(gCtx)

		// Start cloud lifecycle email worker. The worker is a no-op in non-billing builds.
		cloudLifecycleWorker := worker.NewCloudLifecycleWorker(tenantMgr, mailSvc, conf)
		go cloudLifecycleWorker.Start(gCtx)

		// Start cloud retention sync worker (daily reconciliation safety net
		// for the webhook-triggered sync in internal/server/cloud). No-op in
		// non-billing builds.
		cloudRetentionSyncWorker := worker.NewCloudRetentionSyncWorker(tenantMgr, entitlements.NewService(store, ent, conf), conf)
		go cloudRetentionSyncWorker.Start(gCtx)

		startSearchConsoleSyncWorker(gCtx, conf, tenantMgr)

		g.Go(func() error {
			select {
			case <-gCtx.Done():
				return nil
			case fatalErr := <-tenantMgr.FatalErrors():
				return fatalErr
			}
		})

		g.Go(func() error {
			<-gCtx.Done()
			leaderShutdown()
			return nil
		})
	} else {
		slog.Debug("Node is a follower, skipping stateful service initialization.")
		if conf.MCPEnabled {
			slog.Info("MCP server is leader-only and will not start on this follower")
		}
	}

	httpServer := server.New(conf, publicFS, store, tenantMgr, ent, clusterManager, producer, mailSvc, realtimeBroker)
	if store != nil {
		importCleanupWorker := worker.NewImportStageCleanupWorker(store, conf.DataPath, conf.ImportStageRetentionDays, httpServer.ImportStageCleanupStatus())
		go importCleanupWorker.Start(gCtx)
	}
	if tenantMgr != nil && conf.BackupPath != "" {
		var backupS3 *worker.S3Config
		if worker.IsS3ArchivePath(conf.BackupPath) {
			backupS3 = &worker.S3Config{
				AccessKeyID:     conf.S3AccessKeyID,
				SecretAccessKey: conf.S3SecretAccessKey,
				SessionToken:    conf.S3SessionToken,
				Region:          conf.S3Region,
				Endpoint:        conf.S3Endpoint,
				URLStyle:        conf.S3URLStyle,
				UseSSL:          conf.S3UseSSL,
			}
		}
		backupWorker := worker.NewBackupWorker(tenantMgr, conf.DataPath, conf.BackupPath,
			conf.BackupIntervalMinutes, conf.BackupRetentionCount, backupS3, httpServer.BackupStatus())
		go backupWorker.Start(gCtx)
	}

	g.Go(func() error {
		slog.Info("HTTP server starting", "addr", conf.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})

	g.Go(func() error {
		<-gCtx.Done()
		slog.Info("Shutdown signal received, shutting down HTTP server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	})

	slog.Info("Application is running. Press Ctrl+C to exit.")

	check(g.Wait())
}

func startSearchConsoleSyncWorker(ctx context.Context, conf *config.Config, tenantMgr *database.TenantStoreManager) {
	if strings.TrimSpace(conf.GoogleSearchConsoleClientID) == "" || strings.TrimSpace(conf.GoogleSearchConsoleClientSecret) == "" {
		return
	}
	searchConsoleWorker := worker.NewSearchConsoleSyncWorker(tenantMgr, searchconsole.NewGoogleClient(searchconsole.OAuthConfig{
		ClientID:     conf.GoogleSearchConsoleClientID,
		ClientSecret: conf.GoogleSearchConsoleClientSecret,
	}))
	go searchConsoleWorker.Start(ctx)
}

func startLeaderServices(ctx context.Context, conf *config.Config, logger *slog.Logger, logLevel slog.Level, realtimeBroker *realtime.Broker) (*controlstore.Store, *database.TenantStoreManager, *nsq.Producer, func(), error) {
	slog.Debug("(Leader) Starting stateful services...")
	if err := validateLiveDatabasePaths(conf); err != nil {
		return nil, nil, nil, nil, err
	}

	openLegacyStore := func(forMandatorySplit bool) (*database.Store, error) {
		opener := database.OpenMigratedStore
		if forMandatorySplit {
			opener = database.OpenDefaultSplitControlStore
		}
		return opener(ctx, conf.DBPath,
			database.WithMemoryLimit(conf.DuckDBMemoryLimit),
			database.WithThreads(conf.DuckDBThreads),
			database.WithCheckpointInterval(time.Duration(conf.DBCheckpointIntervalMinutes)*time.Minute),
			database.WithAutomaticRecovery(conf.DBAutoRecover, conf.DBRecoveryPath),
			database.WithAutomaticWALRecovery(conf.DBAutoRecoverWAL),
		)
	}

	publicationState, err := controlstore.InspectPublication(ctx, conf.DBPath)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if publicationState == controlstore.PublicationNeedsRebuild || publicationState == controlstore.PublicationNeedsWorkRebuild {
		rebuildFromEvidence := publicationState == controlstore.PublicationNeedsRebuild
		if err := controlstore.ResetInvalidLegacyWork(ctx, conf.DBPath); err != nil {
			return nil, nil, nil, nil, err
		}
		sourcePath := conf.DBPath
		if rebuildFromEvidence {
			sourcePath = controlstore.PreSQLiteEvidencePath(conf.DBPath)
		}
		source, sourceSHA, schemaSHA, sourceErr := database.OpenLegacyControlSource(ctx, sourcePath)
		if sourceErr != nil {
			return nil, nil, nil, nil, sourceErr
		}
		_, importErr := controlstore.ImportLegacy(ctx, controlstore.SQLiteWorkPath(conf.DBPath), source, sourceSHA, schemaSHA)
		closeErr := source.Close()
		if importErr != nil {
			return nil, nil, nil, nil, importErr
		}
		if closeErr != nil {
			return nil, nil, nil, nil, closeErr
		}
		if err := controlstore.PublishLegacyConversion(ctx, conf.DBPath); err != nil {
			return nil, nil, nil, nil, err
		}
		publicationState = controlstore.PublicationSQLiteReady
	}
	if publicationState == controlstore.PublicationCanComplete {
		if err := controlstore.PublishLegacyConversion(ctx, conf.DBPath); err != nil {
			return nil, nil, nil, nil, err
		}
		publicationState = controlstore.PublicationSQLiteReady
	}
	if publicationState == controlstore.PublicationNeedsConversion {
		legacy, openErr := openLegacyStore(true)
		if openErr != nil {
			return nil, nil, nil, nil, openErr
		}
		if legacy.RecoveredDuringConnect() {
			_ = legacy.Close()
			return nil, nil, nil, nil, fmt.Errorf("automatic DuckDB recovery completed; restart HitKeep to continue the mandatory 2.13 migration from a clean startup")
		}
		complete, markerErr := legacy.DefaultTenantSplitComplete(ctx)
		if markerErr != nil {
			_ = legacy.Close()
			return nil, nil, nil, nil, markerErr
		}
		if !complete {
			if err := legacy.Close(); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("close DuckDB control database before default tenant split: %w", err)
			}
			if err := database.RunDefaultTenantSplit(ctx, conf.DBPath, conf.DataPath,
				database.WithMemoryLimit(conf.DuckDBMemoryLimit),
				database.WithThreads(conf.DuckDBThreads),
			); err != nil {
				return nil, nil, nil, nil, err
			}
			legacy, err = openLegacyStore(false)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("reopen DuckDB control database after default tenant split: %w", err)
			}
		}
		if err := legacy.Close(); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("close legacy DuckDB control database: %w", err)
		}
		if err := database.DrainLegacyTenantAnalytics(ctx, conf.DBPath, conf.DataPath,
			database.WithMemoryLimit(conf.DuckDBMemoryLimit),
			database.WithThreads(conf.DuckDBThreads),
		); err != nil {
			return nil, nil, nil, nil, err
		}
		workPath := controlstore.SQLiteWorkPath(conf.DBPath)
		if _, statErr := os.Stat(workPath); errors.Is(statErr, os.ErrNotExist) {
			source, sourceSHA, schemaSHA, sourceErr := database.OpenLegacyControlSource(ctx, conf.DBPath)
			if sourceErr != nil {
				return nil, nil, nil, nil, sourceErr
			}
			_, importErr := controlstore.ImportLegacy(ctx, workPath, source, sourceSHA, schemaSHA)
			closeErr := source.Close()
			if importErr != nil {
				return nil, nil, nil, nil, importErr
			}
			if closeErr != nil {
				return nil, nil, nil, nil, closeErr
			}
		} else if statErr != nil {
			return nil, nil, nil, nil, fmt.Errorf("inspect SQLite conversion work file: %w", statErr)
		}
		if err := controlstore.PublishLegacyConversion(ctx, conf.DBPath); err != nil {
			return nil, nil, nil, nil, err
		}
	}
	store, err := controlstore.Open(ctx, conf.DBPath)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if _, err := store.EnsureDefaultTenant(ctx); err != nil {
		_ = store.Close()
		return nil, nil, nil, nil, fmt.Errorf("ensure default control tenant: %w", err)
	}

	storeOptions := []database.StoreOption{
		database.WithMemoryLimit(conf.DuckDBMemoryLimit),
		database.WithThreads(conf.DuckDBThreads),
		database.WithCheckpointInterval(time.Duration(conf.DBCheckpointIntervalMinutes) * time.Minute),
		database.WithAutomaticRecovery(conf.DBAutoRecover, conf.DBRecoveryPath),
		database.WithAutomaticWALRecovery(conf.DBAutoRecoverWAL),
	}
	var tenantOpts []database.TenantStoreManagerOption
	if conf.DBCompactOnStart {
		compaction := database.DefaultCompactionOptions()
		compaction.MemoryLimit = conf.DuckDBMemoryLimit
		compaction.Threads = conf.DuckDBThreads
		tenantOpts = append(tenantOpts, database.WithTenantCompaction(compaction))
	}
	tenantMgr := database.NewTenantStoreManager(store, conf.DataPath, storeOptions, tenantOpts...)
	if _, err := tenantMgr.ForTenant(ctx, uuid.Nil); err != nil {
		_ = store.Close()
		return nil, nil, nil, nil, fmt.Errorf("open default tenant data-plane root: %w", err)
	}
	closeStores := func() {
		if err := tenantMgr.Close(); err != nil {
			slog.Error("Failed to close tenant databases during startup cleanup", "error", err)
		}
		if err := store.Close(); err != nil {
			slog.Error("Failed to close SQLite control database during startup cleanup", "error", err)
		}
	}
	tenantMgr.StartMaintenance(ctx)
	store.StartMaintenance(ctx, time.Duration(conf.DBCheckpointIntervalMinutes)*time.Minute)
	if err := tenantMgr.SyncAllTenants(ctx); err != nil {
		closeStores()
		return nil, nil, nil, nil, err
	}

	nsqdOpts := nsqd.NewOptions()
	tmpDir, _ := os.MkdirTemp("", "nsqd")
	nsqdOpts.DataPath = tmpDir

	// Use configured internal addresses
	nsqdOpts.TCPAddress = conf.NSQTCPAddress
	nsqdOpts.HTTPAddress = conf.NSQHTTPAddress

	// Wire up NSQD logger to slog
	hklog.ApplyNSQDLogger(nsqdOpts, logger, logLevel)

	nsqdServer, err := nsqd.New(nsqdOpts)
	if err != nil {
		closeStores()
		return nil, nil, nil, nil, err
	}

	go func() {
		if err := nsqdServer.Main(); err != nil {
			slog.Error("Embedded NSQD server exited", "error", err)
		}
	}()
	// Listen for context cancellation to gracefully shut down NSQD.
	go func() {
		<-ctx.Done()
		nsqdServer.Exit()
	}()
	// Producer connects to the local embedded NSQ
	producer, err := nsq.NewProducer(conf.NSQTCPAddress, nsq.NewConfig())
	if err != nil {
		closeStores()
		return nil, nil, nil, nil, err
	}
	// Wire up Producer logger to slog
	producer.SetLogger(hklog.GoNSQLogger{Logger: logger}, hklog.NSQGoLevel(logLevel))

	// The embedded nsqd starts asynchronously; wait until it accepts
	// connections instead of racing it with a fixed sleep.
	var pingErr error
	for range 50 {
		if pingErr = producer.Ping(); pingErr == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if pingErr != nil {
		producer.Stop()
		closeStores()
		return nil, nil, nil, nil, fmt.Errorf("embedded nsqd did not become ready: %w", pingErr)
	}

	consumer := ingest.NewConsumer(tenantMgr, logger, logLevel, realtimeBroker)
	consumer.SetWebhookEmitter(webhookdispatcher.NewEmitter(store, producer, conf.Version))
	if err := consumer.Connect(conf.NSQTCPAddress); err != nil {
		producer.Stop()
		closeStores()
		return nil, nil, nil, nil, err
	}
	webhookWorker := webhookdispatcher.NewWorker(store, producer, *conf, logger, logLevel)
	if err := webhookWorker.Connect(ctx, conf.NSQTCPAddress); err != nil {
		producer.Stop()
		consumer.Stop()
		closeStores()
		return nil, nil, nil, nil, err
	}

	shutdownFunc := func() {
		slog.Debug("(Leader) Shutting down stateful services...")
		webhookWorker.Stop()
		consumer.Stop()
		producer.Stop()
		if err := tenantMgr.Close(); err != nil {
			slog.Error("Failed to close tenant databases", "error", err)
		}
		if err := store.Close(); err != nil {
			slog.Error("Failed to close SQLite control database cleanly", "error", err)
		}
		os.RemoveAll(tmpDir)
	}

	return store, tenantMgr, producer, shutdownFunc, nil
}

func validateLiveDatabasePaths(conf *config.Config) error {
	if worker.IsS3ArchivePath(conf.DBPath) {
		return fmt.Errorf("HITKEEP_DB_PATH must be a local writable SQLite control path; use HITKEEP_BACKUP_PATH for S3 snapshots")
	}
	if worker.IsS3ArchivePath(conf.DataPath) {
		return fmt.Errorf("HITKEEP_DATA_PATH must be a local writable directory; use HITKEEP_BACKUP_PATH for S3 snapshots")
	}
	return nil
}

func runHealthcheck(conf *config.Config) error {
	_, port, err := net.SplitHostPort(conf.HTTPAddr)
	if err != nil {
		port = "8080"
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/healthz", port)

	transport := &http.Transport{
		DisableKeepAlives: true,
	}
	defer transport.CloseIdleConnections()

	client := http.Client{
		Timeout:   2 * time.Second,
		Transport: transport,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return fmt.Errorf("build healthcheck request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("healthcheck request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	return nil
}
