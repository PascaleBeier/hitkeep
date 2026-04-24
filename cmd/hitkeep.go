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
	"syscall"
	"time"

	"github.com/nsqio/go-nsq"
	"github.com/nsqio/nsq/nsqd"
	"golang.org/x/sync/errgroup"

	"hitkeep/internal/cluster"
	"hitkeep/internal/config"
	"hitkeep/internal/database"
	"hitkeep/internal/entitlements"
	"hitkeep/internal/hklog"
	"hitkeep/internal/ingest"
	"hitkeep/internal/mailer"
	"hitkeep/internal/server"
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

	var store *database.Store
	var tenantMgr *database.TenantStoreManager
	var producer *nsq.Producer
	ent := entitlements.NewProvider(conf)

	if clusterManager.IsLeader() {
		var leaderShutdown func()

		store, tenantMgr, producer, leaderShutdown, err = startLeaderServices(gCtx, conf, logger, logLevel)
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
		retentionWorker := worker.NewRetentionWorker(tenantMgr, conf.ArchivePath, conf.DataRetentionDays, s3Conf)
		go retentionWorker.Start(gCtx)

		// Start Rollup Backfill Worker
		rollupWorker := worker.NewRollupBackfillWorker(tenantMgr)
		go rollupWorker.Start(gCtx)

		// Start Report Worker
		reportWorker := worker.NewReportWorker(tenantMgr, mailSvc, conf.PublicURL)
		go reportWorker.Start(gCtx)

		// Start Backup Worker
		if conf.BackupPath != "" {
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
				conf.BackupIntervalMinutes, conf.BackupRetentionCount, backupS3)
			go backupWorker.Start(gCtx)
		}

		g.Go(func() error {
			<-gCtx.Done()
			tenantMgr.Close()
			leaderShutdown()
			return nil
		})
	} else {
		slog.Debug("Node is a follower, skipping stateful service initialization.")
		if conf.MCPEnabled {
			slog.Info("MCP server is leader-only and will not start on this follower")
		}
	}

	httpServer := server.New(conf, publicFS, store, tenantMgr, ent, clusterManager, producer, mailSvc)

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

func startLeaderServices(ctx context.Context, conf *config.Config, logger *slog.Logger, logLevel slog.Level) (*database.Store, *database.TenantStoreManager, *nsq.Producer, func(), error) {
	slog.Debug("(Leader) Starting stateful services...")

	store := database.NewStore(conf.DBPath)
	if err := store.Connect(); err != nil {
		return nil, nil, nil, nil, err
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		return nil, nil, nil, nil, err
	}
	store.StartMaintenance(ctx)

	tenantMgr := database.NewTenantStoreManager(store, conf.DataPath)
	if err := tenantMgr.SyncAllTenants(ctx); err != nil {
		store.Close()
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
		store.Close()
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
	time.Sleep(100 * time.Millisecond)

	// Producer connects to the local embedded NSQ
	producer, err := nsq.NewProducer(conf.NSQTCPAddress, nsq.NewConfig())
	if err != nil {
		store.Close()
		return nil, nil, nil, nil, err
	}
	// Wire up Producer logger to slog
	producer.SetLogger(hklog.GoNSQLogger{Logger: logger}, hklog.NSQGoLevel(logLevel))

	consumer := ingest.NewConsumer(tenantMgr, logger, logLevel)
	if err := consumer.Connect(conf.NSQTCPAddress); err != nil {
		producer.Stop()
		store.Close()
		return nil, nil, nil, nil, err
	}

	shutdownFunc := func() {
		slog.Debug("(Leader) Shutting down stateful services...")
		producer.Stop()
		consumer.Stop()
		store.Close()
		os.RemoveAll(tmpDir)
	}

	return store, tenantMgr, producer, shutdownFunc, nil
}

func runHealthcheck(conf *config.Config) error {
	_, port, err := net.SplitHostPort(conf.HTTPAddr)
	if err != nil {
		port = "8080"
	}

	url := fmt.Sprintf("http://127.0.0.1:%s/healthz", port)

	client := http.Client{
		Timeout: 2 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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
