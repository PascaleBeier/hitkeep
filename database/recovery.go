package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"

	"hitkeep/hklog"
	json "hitkeep/jsonapi"
)

const recoveryMarkerVersion = 1

type recoveryOptions struct {
	enabled              bool
	automaticWALRecovery bool
	root                 string
}

type databaseRecovery struct {
	store  *Store
	opts   recoveryOptions
	status *databaseStatusTracker
}

type recoveryArtifact struct {
	Name         string `json:"name"`
	OriginalSize int64  `json:"original_size"`
	SHA256       string `json:"sha256"`
}

type recoveryBundleManifest struct {
	Version    int                `json:"version"`
	DatabaseID string             `json:"database_id"`
	Trigger    string             `json:"trigger"`
	CreatedAt  time.Time          `json:"created_at"`
	Artifacts  []recoveryArtifact `json:"artifacts"`
}

type recoveryMarker struct {
	Version              int       `json:"version"`
	DatabaseID           string    `json:"database_id"`
	Kind                 string    `json:"kind"`
	Trigger              string    `json:"trigger"`
	Phase                string    `json:"phase"`
	BundleDir            string    `json:"bundle_dir"`
	WALAsidePath         string    `json:"wal_aside_path,omitempty"`
	MigrationWAL         bool      `json:"migration_wal,omitempty"`
	MigrationCheckpoint  string    `json:"migration_checkpoint,omitempty"`
	MigrationDBSHA256    string    `json:"migration_database_sha256,omitempty"`
	RepairTable          string    `json:"repair_table,omitempty"`
	RepairIndexes        []string  `json:"repair_indexes,omitempty"`
	RemovedUnsafeIndexes int       `json:"removed_unsafe_indexes,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
}

type recoveryResult struct {
	Version              int       `json:"version"`
	Status               string    `json:"status"`
	CompletedAt          time.Time `json:"completed_at"`
	Trigger              string    `json:"trigger"`
	RemovedUnsafeIndexes int       `json:"removed_unsafe_indexes"`
}

var (
	errAutomaticWALRecoveryDisabled = errors.New("automatic WAL bypass is disabled")
	errNoRepairableIndexes          = errors.New("no non-unique secondary indexes matched the failed mutation")
)

func newDatabaseRecovery(store *Store, opts recoveryOptions, status *databaseStatusTracker) *databaseRecovery {
	return &databaseRecovery{store: store, opts: opts, status: status}
}

func (r *databaseRecovery) enabled() bool {
	return r != nil && r.opts.enabled && r.store != nil && r.store.path != "" && r.store.path != ":memory:"
}

func (r *databaseRecovery) available() bool {
	return r != nil && r.store != nil && r.store.path != "" && r.store.path != ":memory:"
}

func (r *databaseRecovery) recoverStartup(ctx context.Context) error {
	if !r.available() {
		return nil
	}
	marker, err := r.loadMarker()
	if err != nil {
		return err
	}
	if marker == nil {
		return nil
	}
	hklog.LoggerFromContextOr(ctx, r.store.logger).Warn("Resuming interrupted DuckDB recovery", "database_id", marker.DatabaseID, "kind", marker.Kind, "phase", marker.Phase)
	switch marker.Kind {
	case "remove_unsafe_indexes":
		return r.applyIndexRepair(ctx, marker)
	case "wal_bypass":
		return r.applyWALRecovery(ctx, marker)
	default:
		return fmt.Errorf("unsupported database recovery marker kind %q", marker.Kind)
	}
}

func (r *databaseRecovery) recoverInvalidation(ctx context.Context, trigger error) error {
	if r.available() {
		marker, err := r.loadMarker()
		if err != nil {
			return err
		}
		if marker != nil {
			return r.recoverStartup(ctx)
		}
	}
	if !isIndexMutationCorruption(trigger) {
		r.status.healthy()
		return nil
	}
	if !r.enabled() {
		r.status.recoveryFailed("index_mutation_corruption", "automatic_recovery_disabled")
		return fmt.Errorf("automatic database recovery is disabled")
	}
	return r.repairIndexes(ctx, "index_mutation_corruption", repairTableFromError(trigger))
}

func isIndexMutationCorruption(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "failed to delete all rows from index")
}

func isKnownWALReplayError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "wal") &&
		strings.Contains(message, "replayalter") &&
		strings.Contains(message, "binddefaultvalues") &&
		strings.Contains(message, "getdefaultdatabase")
}

func (r *databaseRecovery) repairIndexes(ctx context.Context, trigger, repairTable string) (retErr error) {
	if !r.enabled() {
		return nil
	}
	r.status.recovering("bundle_database", trigger)
	defer func() {
		if retErr != nil {
			r.status.recoveryFailed(trigger, "remove_unsafe_indexes")
		}
	}()

	bundleDir, err := r.createBundle(trigger)
	if err != nil {
		return err
	}
	marker := &recoveryMarker{
		Version:     recoveryMarkerVersion,
		DatabaseID:  r.databaseID(),
		Kind:        "remove_unsafe_indexes",
		Trigger:     trigger,
		Phase:       "drop_unsafe_indexes",
		BundleDir:   bundleDir,
		RepairTable: repairTable,
		CreatedAt:   time.Now().UTC(),
	}
	if err := r.writeMarker(marker); err != nil {
		return err
	}
	return r.applyIndexRepair(ctx, marker)
}

func (r *databaseRecovery) applyIndexRepair(ctx context.Context, marker *recoveryMarker) (retErr error) {
	if marker == nil {
		return errors.New("missing index recovery marker")
	}
	trigger := marker.Trigger
	r.status.recovering(marker.Phase, trigger)
	defer func() {
		if retErr != nil {
			r.status.recoveryFailed(trigger, marker.Phase)
		}
	}()

	db, err := r.store.openRawDB(ctx)
	if err != nil {
		return fmt.Errorf("open database for index recovery: %w", err)
	}
	defer func() { _ = db.Close() }()

	switch marker.Phase {
	case "", "drop_unsafe_indexes":
		marker.Phase = "drop_unsafe_indexes"
		if len(marker.RepairIndexes) == 0 {
			indexes, err := listRepairableIndexes(ctx, db, marker.RepairTable)
			if err != nil {
				return err
			}
			if len(indexes) == 0 && marker.RemovedUnsafeIndexes == 0 {
				return errNoRepairableIndexes
			}
			if len(indexes) > 0 {
				marker.RepairIndexes = indexes
				marker.RemovedUnsafeIndexes = max(marker.RemovedUnsafeIndexes, len(indexes))
			}
		}
		if err := r.writeMarker(marker); err != nil {
			return err
		}
		for _, name := range marker.RepairIndexes {
			if _, err := db.ExecContext(ctx, "DROP INDEX IF EXISTS "+quoteDuckDBIdentifier(name)); err != nil {
				return fmt.Errorf("drop unsafe database index: %w", err)
			}
		}
		marker.Phase = "checkpoint"
		if err := r.writeMarker(marker); err != nil {
			return err
		}
	case "checkpoint":
	default:
		return fmt.Errorf("unsupported index recovery phase %q", marker.Phase)
	}
	if marker.RemovedUnsafeIndexes == 0 {
		return errNoRepairableIndexes
	}

	r.status.recovering("checkpoint", trigger)
	if _, err := db.ExecContext(ctx, "CHECKPOINT;"); err != nil {
		return fmt.Errorf("checkpoint recovered database: %w", err)
	}
	if err := db.Close(); err != nil {
		return fmt.Errorf("close recovered database: %w", err)
	}

	removedUnsafeIndexes := marker.RemovedUnsafeIndexes
	if err := r.complete(marker, removedUnsafeIndexes); err != nil {
		return err
	}
	r.status.recovered(removedUnsafeIndexes)
	hklog.LoggerFromContextOr(ctx, r.store.logger).Info("DuckDB unsafe-index recovery completed",
		"database_id", marker.DatabaseID,
		"trigger", trigger,
		"removed_unsafe_indexes", removedUnsafeIndexes)
	return nil
}

func listRepairableIndexes(ctx context.Context, db *sql.DB, targetTable string) ([]string, error) {
	targetTable = strings.TrimSpace(targetTable)

	rows, err := db.QueryContext(ctx, `
		SELECT table_name, index_name
		FROM duckdb_indexes()
		WHERE database_name = current_database()
			AND schema_name = 'main'
			AND NOT is_unique
		ORDER BY index_name`)
	if err != nil {
		return nil, fmt.Errorf("list repairable database indexes: %w", err)
	}
	defer rows.Close()

	indexes := make([]string, 0)
	for rows.Next() {
		var table, index string
		if err := rows.Scan(&table, &index); err != nil {
			return nil, fmt.Errorf("scan repairable database index: %w", err)
		}
		if targetTable != "" && !strings.EqualFold(table, targetTable) {
			continue
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read repairable database indexes: %w", err)
	}
	return indexes, nil
}

func (r *databaseRecovery) recoverWAL(ctx context.Context, trigger error) error {
	return r.recoverWALWithPolicy(ctx, trigger, nil)
}

func (r *databaseRecovery) recoverMigrationWAL(ctx context.Context, trigger error, guard *migrationWALGuard) error {
	if guard == nil {
		return errors.New("missing migration WAL guard")
	}
	return r.recoverWALWithPolicy(ctx, trigger, guard)
}

func (r *databaseRecovery) recoverWALWithPolicy(ctx context.Context, trigger error, migrationGuard *migrationWALGuard) error {
	if !r.available() || !isKnownWALReplayError(trigger) {
		return trigger
	}
	migrationWAL := migrationGuard != nil
	triggerName := "wal_replay_default_binding"
	if migrationWAL {
		triggerName = "migration_wal_replay_default_binding"
	}
	r.status.recovering("bundle_wal", triggerName)

	walPath := r.store.path + ".wal"
	if _, err := os.Stat(walPath); err != nil {
		r.status.recoveryFailed(triggerName, "bundle_wal")
		return fmt.Errorf("known WAL replay failure has no WAL file: %w", err)
	}
	bundleDir := ""
	checksumVerified := false
	if migrationGuard != nil && migrationGuard.DatabaseSHA256 != "" {
		actual, err := databaseFileSHA256(r.store.path)
		if err != nil {
			r.status.recoveryFailed(triggerName, "verify_database_checksum")
			return fmt.Errorf("verify checksum-guarded migration WAL: %w", err)
		}
		if actual != migrationGuard.DatabaseSHA256 {
			r.status.recoveryFailed(triggerName, "verify_database_checksum")
			return errors.New("checksum-guarded migration WAL does not match the durable database file")
		}
		checksumVerified = true
	}
	if !checksumVerified {
		var err error
		bundleDir, err = r.createBundle(triggerName)
		if err != nil {
			r.status.recoveryFailed(triggerName, "bundle_wal")
			return err
		}
	}
	asidePath := walPath + ".recovery-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	marker := &recoveryMarker{
		Version:      recoveryMarkerVersion,
		DatabaseID:   r.databaseID(),
		Kind:         "wal_bypass",
		Trigger:      triggerName,
		Phase:        "awaiting_operator",
		BundleDir:    bundleDir,
		WALAsidePath: asidePath,
		MigrationWAL: migrationWAL,
		CreatedAt:    time.Now().UTC(),
	}
	if migrationGuard != nil {
		marker.MigrationCheckpoint = migrationGuard.CheckpointToken
		marker.MigrationDBSHA256 = migrationGuard.DatabaseSHA256
	}
	if err := r.writeMarker(marker); err != nil {
		r.status.recoveryFailed(triggerName, "write_marker")
		return err
	}
	return r.applyWALRecovery(ctx, marker)
}

func (r *databaseRecovery) applyWALRecovery(ctx context.Context, marker *recoveryMarker) (retErr error) {
	if marker == nil {
		return errors.New("missing WAL recovery marker")
	}
	walPath := r.store.path + ".wal"
	asidePath := marker.WALAsidePath
	r.status.recovering(marker.Phase, marker.Trigger)
	defer func() {
		if retErr != nil {
			r.status.recoveryFailed(marker.Trigger, marker.Phase)
		}
	}()

	switch marker.Phase {
	case "awaiting_operator":
		if !fileExists(walPath) {
			if err := r.complete(marker, 0); err != nil {
				return err
			}
			r.status.recovered(0)
			hklog.LoggerFromContextOr(ctx, r.store.logger).Info("DuckDB WAL recovery completed after operator removed the replay-failing WAL", "database_id", marker.DatabaseID)
			return nil
		}
		if !marker.MigrationWAL && !r.opts.automaticWALRecovery {
			marker.Phase = "operator_action_required"
			return fmt.Errorf(
				"%w after retaining recovery bundle; set HITKEEP_DB_AUTO_RECOVER_WAL=true only if accepting loss of WAL-only changes",
				errAutomaticWALRecoveryDisabled,
			)
		}
		marker.Phase = "move_wal"
		if err := r.writeMarker(marker); err != nil {
			return err
		}
		fallthrough
	case "", "move_wal":
		originalExists := fileExists(walPath)
		asideExists := fileExists(asidePath)
		if originalExists && asideExists {
			return errors.New("both live and recovery WAL files exist; refusing automatic recovery")
		}
		if originalExists {
			if err := os.Rename(walPath, asidePath); err != nil {
				return fmt.Errorf("move replay-failing WAL aside: %w", err)
			}
			if err := syncParentDirectory(walPath); err != nil {
				return fmt.Errorf("sync replay-failing WAL move: %w", err)
			}
			asideExists = true
		}
		if !asideExists {
			return errors.New("replay-failing WAL is missing from both live and recovery paths")
		}
		marker.Phase = "checkpoint_without_wal"
		if err := r.writeMarker(marker); err != nil {
			return err
		}
		fallthrough
	case "checkpoint_without_wal":
		if !fileExists(asidePath) {
			return errors.New("replay-failing WAL is missing before the recovery checkpoint")
		}
		r.status.recovering("checkpoint_without_wal", marker.Trigger)
		db, err := r.store.openRawDB(ctx)
		if err != nil {
			return fmt.Errorf("open database without replay-failing WAL: %w", err)
		}
		if marker.MigrationWAL {
			var verifyErr error
			if marker.MigrationDBSHA256 != "" {
				var actual string
				actual, verifyErr = databaseFileSHA256(r.store.path)
				if verifyErr == nil && actual != marker.MigrationDBSHA256 {
					verifyErr = errors.New("migration database checksum mismatch")
				}
			} else {
				verifyErr = r.verifyMigrationCheckpoint(ctx, db, marker.MigrationCheckpoint)
			}
			if verifyErr != nil {
				_ = db.Close()
				if !r.opts.automaticWALRecovery {
					return r.restoreUnverifiedMigrationWAL(marker, walPath, asidePath, verifyErr)
				}
				hklog.LoggerFromContextOr(ctx, r.store.logger).Warn("Migration WAL checkpoint verification failed; continuing because broad WAL recovery is explicitly enabled",
					"database_id", marker.DatabaseID,
					"error", verifyErr)
				marker.MigrationWAL = false
				marker.MigrationCheckpoint = ""
				marker.MigrationDBSHA256 = ""
				if err := r.writeMarker(marker); err != nil {
					return err
				}
				db, err = r.store.openRawDB(ctx)
				if err != nil {
					return fmt.Errorf("reopen database after migration checkpoint verification: %w", err)
				}
			}
		}
		if _, err := db.ExecContext(ctx, "CHECKPOINT;"); err != nil {
			_ = db.Close()
			return fmt.Errorf("checkpoint database after WAL bypass: %w", err)
		}
		if err := db.Close(); err != nil {
			return fmt.Errorf("close database after WAL bypass: %w", err)
		}
		marker.Phase = "cleanup_wal"
		if err := r.writeMarker(marker); err != nil {
			return err
		}
		fallthrough
	case "cleanup_wal":
		r.status.recovering("cleanup_wal", marker.Trigger)
		if err := os.Remove(asidePath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove replay-failing WAL after retained bundle was completed: %w", err)
		}
		if err := syncParentDirectory(asidePath); err != nil {
			return fmt.Errorf("sync replay-failing WAL cleanup: %w", err)
		}
	default:
		return fmt.Errorf("unsupported WAL recovery phase %q", marker.Phase)
	}
	if err := r.complete(marker, 0); err != nil {
		return err
	}
	r.status.recovered(0)
	hklog.LoggerFromContextOr(ctx, r.store.logger).Info("DuckDB WAL recovery completed", "database_id", marker.DatabaseID, "trigger", marker.Trigger)
	return nil
}

func (r *databaseRecovery) restoreUnverifiedMigrationWAL(marker *recoveryMarker, walPath, asidePath string, verifyErr error) error {
	if fileExists(walPath) {
		return fmt.Errorf("migration checkpoint verification failed and live WAL already exists: %w", verifyErr)
	}
	if err := os.Rename(asidePath, walPath); err != nil {
		return fmt.Errorf("restore unverified migration WAL: %w", errors.Join(verifyErr, err))
	}
	if err := syncParentDirectory(walPath); err != nil {
		return fmt.Errorf("sync restored unverified migration WAL: %w", errors.Join(verifyErr, err))
	}
	marker.MigrationWAL = false
	marker.MigrationCheckpoint = ""
	marker.Phase = "awaiting_operator"
	if err := r.writeMarker(marker); err != nil {
		return fmt.Errorf("record rejected migration WAL guard: %w", errors.Join(verifyErr, err))
	}
	return fmt.Errorf(
		"%w because the migration checkpoint identity did not match; the live WAL was restored unchanged: %v",
		errAutomaticWALRecoveryDisabled,
		verifyErr,
	)
}

func (r *databaseRecovery) createBundle(trigger string) (string, error) {
	root := r.root()
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("create database recovery directory: %w", err)
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", fmt.Errorf("secure database recovery directory: %w", err)
	}
	if err := ensureRecoverySpace(root, r.store.path); err != nil {
		return "", err
	}

	name := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + r.databaseID()
	tempDir := filepath.Join(root, "."+name+".tmp")
	finalDir := filepath.Join(root, name)
	if err := os.Mkdir(tempDir, 0o700); err != nil {
		return "", fmt.Errorf("create database recovery bundle: %w", err)
	}
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.RemoveAll(tempDir)
		}
	}()

	paths := []struct {
		source string
		name   string
	}{
		{source: r.store.path, name: "database.zst"},
		{source: r.store.path + ".wal", name: "wal.zst"},
	}
	artifacts := make([]recoveryArtifact, 0, len(paths))
	for _, item := range paths {
		if !fileExists(item.source) {
			continue
		}
		artifact, err := compressRecoveryArtifact(item.source, filepath.Join(tempDir, item.name), item.name)
		if err != nil {
			return "", err
		}
		artifacts = append(artifacts, artifact)
	}
	if len(artifacts) == 0 {
		return "", errors.New("database recovery bundle has no source artifacts")
	}

	manifest := recoveryBundleManifest{
		Version:    recoveryMarkerVersion,
		DatabaseID: r.databaseID(),
		Trigger:    trigger,
		CreatedAt:  time.Now().UTC(),
		Artifacts:  artifacts,
	}
	if err := writeJSONFile(filepath.Join(tempDir, "manifest.json"), manifest); err != nil {
		return "", err
	}
	if err := os.Rename(tempDir, finalDir); err != nil {
		return "", fmt.Errorf("activate database recovery bundle: %w", err)
	}
	if err := syncParentDirectory(finalDir); err != nil {
		return "", fmt.Errorf("sync database recovery bundle: %w", err)
	}
	keepTemp = false
	r.status.bundleAvailable()
	r.store.logger.Info("Created DuckDB recovery bundle",
		"database_id", manifest.DatabaseID,
		"trigger", trigger,
		"artifact_count", len(artifacts))
	return finalDir, nil
}

func compressRecoveryArtifact(sourcePath, targetPath, name string) (recoveryArtifact, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return recoveryArtifact{}, fmt.Errorf("open database recovery source: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return recoveryArtifact{}, fmt.Errorf("stat database recovery source: %w", err)
	}
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return recoveryArtifact{}, fmt.Errorf("create compressed database recovery artifact: %w", err)
	}
	defer target.Close()

	encoder, err := zstd.NewWriter(target, zstd.WithEncoderConcurrency(1))
	if err != nil {
		return recoveryArtifact{}, fmt.Errorf("create database recovery compressor: %w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(encoder, io.TeeReader(source, hash))
	closeErr := encoder.Close()
	if copyErr != nil {
		return recoveryArtifact{}, fmt.Errorf("compress database recovery artifact: %w", copyErr)
	}
	if closeErr != nil {
		return recoveryArtifact{}, fmt.Errorf("finish database recovery artifact: %w", closeErr)
	}
	if err := target.Sync(); err != nil {
		return recoveryArtifact{}, fmt.Errorf("sync database recovery artifact: %w", err)
	}
	return recoveryArtifact{
		Name:         name,
		OriginalSize: info.Size(),
		SHA256:       hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func (r *databaseRecovery) complete(marker *recoveryMarker, removedUnsafeIndexes int) error {
	if marker == nil {
		return errors.New("missing completed recovery marker")
	}
	result := recoveryResult{
		Version:              recoveryMarkerVersion,
		Status:               "completed",
		CompletedAt:          time.Now().UTC(),
		Trigger:              marker.Trigger,
		RemovedUnsafeIndexes: removedUnsafeIndexes,
	}
	if marker.BundleDir != "" {
		if err := writeJSONFile(filepath.Join(marker.BundleDir, "result.json"), result); err != nil {
			return err
		}
	}
	if err := os.Remove(r.markerPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("clear database recovery marker: %w", err)
	}
	if err := syncParentDirectory(r.markerPath()); err != nil {
		return fmt.Errorf("sync cleared database recovery marker: %w", err)
	}
	return nil
}

func (r *databaseRecovery) loadRecoveryHistory() {
	if !r.available() {
		return
	}
	entries, err := os.ReadDir(r.root())
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		r.store.logger.Debug("Could not inspect DuckDB recovery history", "database_id", r.databaseID(), "error", err)
		return
	}

	databaseID := r.databaseID()
	bundleAvailable := false
	var latest *recoveryResult
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasSuffix(entry.Name(), "-"+databaseID) {
			continue
		}
		dir := filepath.Join(r.root(), entry.Name())
		manifestData, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			continue
		}
		var manifest recoveryBundleManifest
		if json.Unmarshal(manifestData, &manifest) != nil || manifest.DatabaseID != databaseID {
			continue
		}
		bundleAvailable = true

		resultData, err := os.ReadFile(filepath.Join(dir, "result.json"))
		if err != nil {
			continue
		}
		var result recoveryResult
		if json.Unmarshal(resultData, &result) != nil || result.Status != "completed" {
			continue
		}
		if latest == nil || result.CompletedAt.After(latest.CompletedAt) {
			copy := result
			latest = &copy
		}
	}

	if latest == nil {
		r.status.restoreRecoveryHistory(bundleAvailable, nil, 0)
		return
	}
	r.status.restoreRecoveryHistory(
		bundleAvailable,
		&latest.CompletedAt,
		latest.RemovedUnsafeIndexes,
	)
}

func (r *databaseRecovery) root() string {
	if root := strings.TrimSpace(r.opts.root); root != "" {
		return root
	}
	return filepath.Join(filepath.Dir(r.store.path), "recovery")
}

func (r *databaseRecovery) databaseID() string {
	path, err := filepath.Abs(r.store.path)
	if err != nil {
		path = r.store.path
	}
	sum := sha256.Sum256([]byte(path))
	return hex.EncodeToString(sum[:8])
}

func (r *databaseRecovery) markerPath() string {
	return filepath.Join(r.root(), r.databaseID()+".recovery.json")
}

func (r *databaseRecovery) loadMarker() (*recoveryMarker, error) {
	data, err := os.ReadFile(r.markerPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read database recovery marker: %w", err)
	}
	var marker recoveryMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return nil, fmt.Errorf("decode database recovery marker: %w", err)
	}
	if marker.Version != recoveryMarkerVersion || marker.DatabaseID != r.databaseID() {
		return nil, errors.New("database recovery marker does not match this database")
	}
	return &marker, nil
}

func (r *databaseRecovery) writeMarker(marker *recoveryMarker) error {
	if marker == nil {
		return errors.New("missing database recovery marker")
	}
	if err := os.MkdirAll(r.root(), 0o700); err != nil {
		return fmt.Errorf("create database recovery marker directory: %w", err)
	}
	return writeJSONFile(r.markerPath(), marker)
}

func writeJSONFile(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode database recovery metadata: %w", err)
	}
	tempPath := path + ".tmp"
	file, err := os.OpenFile(tempPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create database recovery metadata: %w", err)
	}
	if _, err := file.Write(append(data, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("write database recovery metadata: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync database recovery metadata: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close database recovery metadata: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("activate database recovery metadata: %w", err)
	}
	if err := syncParentDirectory(path); err != nil {
		return fmt.Errorf("sync database recovery metadata directory: %w", err)
	}
	return nil
}

func quoteDuckDBIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func databaseFileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
