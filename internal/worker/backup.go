package worker

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"

	"hitkeep/internal/database"
)

// BackupWorker periodically exports all DuckDB databases to Parquet snapshots.
type BackupWorker struct {
	tenantMgr      *database.TenantStoreManager
	listTenantIDs  func(context.Context) ([]uuid.UUID, error)
	dataPath       string
	backupPath     string
	intervalMin    int
	retentionCount int
	s3Config       *S3Config
	status         *database.BackupStatusTracker
}

// NewBackupWorker creates a BackupWorker. If backupPath is empty, Start is a no-op.
func NewBackupWorker(
	tenantMgr *database.TenantStoreManager,
	dataPath string,
	backupPath string,
	intervalMin int,
	retentionCount int,
	s3Config *S3Config,
	status *database.BackupStatusTracker,
) *BackupWorker {
	return &BackupWorker{
		tenantMgr:      tenantMgr,
		listTenantIDs:  tenantMgr.Control().ListActiveTenantIDs,
		dataPath:       dataPath,
		backupPath:     strings.TrimSpace(backupPath),
		intervalMin:    intervalMin,
		retentionCount: retentionCount,
		s3Config:       s3Config,
		status:         status,
	}
}

// Start runs the backup worker on a ticker loop. It returns immediately if
// backupPath is empty (backups disabled).
func (w *BackupWorker) Start(ctx context.Context) {
	if w.backupPath == "" {
		return
	}

	if IsS3ArchivePath(w.backupPath) {
		slog.Info("S3 backup enabled", "path", w.backupPath, "interval_min", w.intervalMin, "retention", w.retentionCount)
	} else {
		slog.Info("Local backup enabled", "path", w.backupPath, "interval_min", w.intervalMin, "retention", w.retentionCount)
	}
	w.setNextBackup(time.Now().UTC().Add(30 * time.Second))

	// Initial run after a short delay to let DB settle.
	go func() {
		time.Sleep(30 * time.Second)
		if err := w.Run(ctx); err != nil {
			slog.Error("Initial backup run failed", "error", err)
		}
	}()

	interval := time.Duration(w.intervalMin) * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.Run(ctx); err != nil {
				slog.Error("Backup worker failed", "error", err)
			}
		}
	}
}

// Run executes a single backup cycle: export shared DB + all tenant DBs,
// then prune old snapshots beyond the retention count.
func (w *BackupWorker) Run(ctx context.Context) (err error) {
	defer func() {
		finishedAt := time.Now().UTC()
		if err != nil {
			w.recordFailure(finishedAt, err)
			return
		}
		w.recordSuccess(finishedAt)
	}()

	timestamp := time.Now().UTC().Format("2006-01-02T150405Z")
	slog.Info("Starting database backup", "timestamp", timestamp)

	isS3 := IsS3ArchivePath(w.backupPath)

	// The compatibility location remains shared/<timestamp>, but its payload is
	// now a compact SQLite control snapshot rather than a DuckDB export.
	sharedDest := joinArchivePath(w.backupPath, "shared", timestamp)
	if err := w.backupControl(ctx, sharedDest, isS3); err != nil {
		w.removeIncompleteLocalSnapshot(sharedDest, isS3)
		return fmt.Errorf("backup shared database: %w", err)
	}
	slog.Info("Control database backed up", "dest", sharedDest)

	// Discover every active tenant, including the default tenant. The shared
	// snapshot above remains control-plane-only; tenant snapshots live under
	// tenants/<tenantID>.
	tenantIDs, err := w.listTenantIDs(ctx)
	if err != nil {
		w.removeIncompleteLocalSnapshot(sharedDest, isS3)
		return fmt.Errorf("list active tenants for complete backup cycle: %w", err)
	}

	// Backup every active tenant database, including the default data-plane root.
	var tenantErrors []error
	for _, tenantID := range tenantIDs {
		tenantStore, err := w.tenantMgr.ForTenant(ctx, tenantID)
		if err != nil {
			slog.Error("Failed to open tenant store for backup", "tenant_id", tenantID, "error", err)
			tenantErrors = append(tenantErrors, fmt.Errorf("open tenant %s database for backup: %w", tenantID, err))
			continue
		}
		tenantDest := joinArchivePath(w.backupPath, "tenants", tenantID.String(), timestamp)
		if err := w.exportDatabase(ctx, tenantStore, tenantDest, isS3); err != nil {
			slog.Error("Failed to backup tenant database", "tenant_id", tenantID, "error", err)
			w.removeIncompleteLocalSnapshot(tenantDest, isS3)
			tenantErrors = append(tenantErrors, fmt.Errorf("backup tenant %s database: %w", tenantID, err))
			continue
		}
		slog.Info("Tenant database backed up", "tenant_id", tenantID, "dest", tenantDest)
	}

	if len(tenantErrors) > 0 {
		w.removeIncompleteLocalSnapshot(sharedDest, isS3)
		return fmt.Errorf("backup incomplete: %w", errors.Join(tenantErrors...))
	}
	if err := w.completeBackupCycle(ctx, sharedDest, isS3); err != nil {
		w.removeIncompleteLocalSnapshot(sharedDest, isS3)
		return fmt.Errorf("publish backup completion marker: %w", err)
	}

	// Never evict an older valid snapshot until the replacement cycle is
	// complete. The shared completion marker is the cycle-wide commit point.
	if !isS3 {
		w.pruneLocalSnapshots(filepath.Join(w.backupPath, "shared"))
		for _, tenantID := range tenantIDs {
			w.pruneLocalSnapshots(filepath.Join(w.backupPath, "tenants", tenantID.String()))
		}
	} else {
		slog.Debug("S3 backup pruning: configure S3 lifecycle policies to manage snapshot retention")
	}

	slog.Info("Database backup completed", "timestamp", timestamp)
	return nil
}

func (w *BackupWorker) backupControl(ctx context.Context, dest string, isS3 bool) error {
	localDest := dest
	cleanup := func() {}
	if isS3 {
		base := strings.TrimSpace(w.dataPath)
		if base == "" {
			base = os.TempDir()
		}
		if err := os.MkdirAll(base, 0o700); err != nil {
			return fmt.Errorf("create temporary control backup base: %w", err)
		}
		temporary, err := os.MkdirTemp(base, ".control-backup-*")
		if err != nil {
			return fmt.Errorf("create temporary control snapshot directory: %w", err)
		}
		localDest = temporary
		cleanup = func() { _ = os.RemoveAll(temporary) }
	}
	defer cleanup()
	if err := os.MkdirAll(localDest, 0o700); err != nil {
		return fmt.Errorf("create control snapshot directory: %w", err)
	}
	snapshot := filepath.Join(localDest, "control.db")
	info, err := w.tenantMgr.Control().Backup(ctx, snapshot)
	if err != nil {
		return err
	}
	compressed := snapshot + ".zst"
	if err := compressControlSnapshot(snapshot, compressed); err != nil {
		return err
	}
	if err := os.Remove(snapshot); err != nil {
		return fmt.Errorf("remove uncompressed control snapshot: %w", err)
	}
	manifest := struct {
		Version int    `json:"version"`
		Engine  string `json:"engine"`
		File    string `json:"file"`
		Bytes   int64  `json:"bytes"`
		SHA256  string `json:"sha256"`
	}{Version: 1, Engine: "sqlite", File: "control.db.zst", Bytes: info.Bytes, SHA256: info.SHA256}
	payload, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(localDest, "manifest.json"), payload, 0o600); err != nil {
		return fmt.Errorf("write control backup manifest: %w", err)
	}
	if isS3 {
		return w.publishControlS3(ctx, dest, compressed, payload)
	}
	return nil
}

func (w *BackupWorker) publishControlS3(ctx context.Context, destination, compressedPath string, manifest []byte) error {
	bucket, prefix, err := parseS3Destination(destination)
	if err != nil {
		return err
	}
	client, err := w.controlS3Client(ctx)
	if err != nil {
		return err
	}
	uploaded := make([]string, 0, 2)
	cleanup := func() {
		for _, key := range uploaded {
			_, _ = client.DeleteObject(context.Background(), &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
		}
	}
	put := func(key string, body io.Reader) error {
		if _, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: body}); err != nil {
			cleanup()
			return err
		}
		uploaded = append(uploaded, key)
		return nil
	}
	compressed, err := os.Open(compressedPath)
	if err != nil {
		return err
	}
	if err := put(joinS3Key(prefix, "control.db.zst"), compressed); err != nil {
		_ = compressed.Close()
		return fmt.Errorf("upload SQLite control snapshot: %w", err)
	}
	if err := compressed.Close(); err != nil {
		cleanup()
		return err
	}
	if err := put(joinS3Key(prefix, "manifest.json"), bytes.NewReader(manifest)); err != nil {
		return fmt.Errorf("upload SQLite control manifest: %w", err)
	}
	return nil
}

func (w *BackupWorker) completeBackupCycle(ctx context.Context, destination string, isS3 bool) error {
	if !isS3 {
		return os.WriteFile(filepath.Join(destination, "_COMPLETE"), []byte("ok\n"), 0o600)
	}
	bucket, prefix, err := parseS3Destination(destination)
	if err != nil {
		return err
	}
	client, err := w.controlS3Client(ctx)
	if err != nil {
		return err
	}
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(joinS3Key(prefix, "_COMPLETE")),
		Body:   strings.NewReader("ok\n"),
	})
	return err
}

func (w *BackupWorker) controlS3Client(ctx context.Context) (*s3.Client, error) {
	region := ""
	if w.s3Config != nil {
		region = w.s3Config.Region
	}
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if w.s3Config != nil && strings.TrimSpace(w.s3Config.AccessKeyID) != "" {
		provider := credentials.NewStaticCredentialsProvider(w.s3Config.AccessKeyID, w.s3Config.SecretAccessKey, w.s3Config.SessionToken)
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(provider))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, fmt.Errorf("load AWS configuration for control backup: %w", err)
	}
	return s3.NewFromConfig(cfg, func(options *s3.Options) {
		if w.s3Config == nil {
			return
		}
		if endpoint := strings.TrimSpace(w.s3Config.Endpoint); endpoint != "" {
			if !strings.Contains(endpoint, "://") {
				scheme := "https"
				if !w.s3Config.UseSSL {
					scheme = "http"
				}
				endpoint = scheme + "://" + endpoint
			}
			options.BaseEndpoint = aws.String(endpoint)
		}
		style := strings.ToLower(strings.TrimSpace(w.s3Config.URLStyle))
		options.UsePathStyle = style == "path" || style == "path_style"
	}), nil
}

func parseS3Destination(destination string) (string, string, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(destination), "s3://")
	bucket, prefix, found := strings.Cut(trimmed, "/")
	if !found || strings.TrimSpace(bucket) == "" || strings.TrimSpace(prefix) == "" {
		return "", "", fmt.Errorf("invalid S3 backup destination")
	}
	return bucket, strings.Trim(prefix, "/"), nil
}

func joinS3Key(prefix, name string) string {
	return strings.Trim(prefix, "/") + "/" + strings.TrimLeft(name, "/")
}

func compressControlSnapshot(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		_ = out.Close()
		if cleanup {
			_ = os.Remove(destination)
		}
	}()
	encoder, err := zstd.NewWriter(out, zstd.WithEncoderConcurrency(1))
	if err != nil {
		return err
	}
	if _, err := io.Copy(encoder, in); err != nil {
		_ = encoder.Close()
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func (w *BackupWorker) removeIncompleteLocalSnapshot(path string, isS3 bool) {
	if isS3 || strings.TrimSpace(path) == "" {
		return
	}
	if err := os.RemoveAll(path); err != nil {
		slog.Warn("Could not remove incomplete backup snapshot", "path", path, "error", err)
	}
}

func (w *BackupWorker) recordSuccess(at time.Time) {
	if w.status == nil {
		return
	}
	w.status.SetLastBackup(at)
	if next, ok := w.nextBackupTime(at); ok {
		w.status.SetNextBackup(next)
	}
}

func (w *BackupWorker) recordFailure(at time.Time, err error) {
	if w.status == nil {
		return
	}
	w.status.SetFailed(at, err.Error())
	if next, ok := w.nextBackupTime(at); ok {
		w.status.SetNextBackup(next)
	}
}

func (w *BackupWorker) setNextBackup(at time.Time) {
	if w.status == nil {
		return
	}
	w.status.SetNextBackup(at)
}

func (w *BackupWorker) nextBackupTime(after time.Time) (time.Time, bool) {
	if w.intervalMin <= 0 {
		return time.Time{}, false
	}
	return after.Add(time.Duration(w.intervalMin) * time.Minute), true
}

// exportDatabase checkpoints and exports a DuckDB database to the given destination.
func (w *BackupWorker) exportDatabase(ctx context.Context, store *database.Store, dest string, isS3 bool) error {
	if store == nil {
		return errors.New("database store is not available")
	}
	if err := store.Checkpoint(ctx, "backup"); err != nil {
		return fmt.Errorf("checkpoint before export: %w", err)
	}
	// Ensure local directory exists.
	if !isS3 {
		if err := os.MkdirAll(dest, 0755); err != nil {
			return fmt.Errorf("create backup directory %s: %w", dest, err)
		}
	}

	safeDest := strings.ReplaceAll(dest, "'", "''")
	query := fmt.Sprintf("EXPORT DATABASE '%s' (FORMAT PARQUET);", safeDest)
	return database.WithDuckDBSession(ctx, store.DB(), database.DuckDBSessionOptions{
		S3: s3ConfigForSession(isS3, w.s3Config),
	}, func(conn *sql.Conn) error {
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("export database to %s: %w", dest, err)
		}
		return nil
	})
}

// pruneLocalSnapshots removes the oldest snapshot directories in dir,
// keeping at most retentionCount. Snapshot dirs are ISO timestamp names
// that sort lexicographically.
func (w *BackupWorker) pruneLocalSnapshots(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("Could not read backup directory for pruning", "dir", dir, "error", err)
		}
		return
	}

	// Remove crash leftovers before retention counting so an incomplete newer
	// directory can never evict an older valid snapshot.
	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if !completeLocalBackupSnapshot(path) {
			if err := os.RemoveAll(path); err != nil {
				slog.Warn("Could not remove incomplete backup snapshot", "path", path, "error", err)
			}
			continue
		}
		dirs = append(dirs, e.Name())
	}

	if len(dirs) <= w.retentionCount {
		return
	}

	sort.Strings(dirs)

	toRemove := dirs[:len(dirs)-w.retentionCount]
	for _, name := range toRemove {
		path := filepath.Join(dir, name)
		if err := os.RemoveAll(path); err != nil {
			slog.Error("Failed to prune old backup snapshot", "path", path, "error", err)
		} else {
			slog.Info("Pruned old backup snapshot", "path", path)
		}
	}
}

func completeLocalBackupSnapshot(path string) bool {
	if info, err := os.Stat(filepath.Join(path, "_COMPLETE")); err == nil && info.Mode().IsRegular() {
		return true
	}
	for _, name := range []string{"load.sql", "schema.sql"} {
		info, err := os.Stat(filepath.Join(path, name))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}
