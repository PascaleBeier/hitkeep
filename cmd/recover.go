package hitkeepcmd

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"

	"hitkeep/config"
	"hitkeep/internal/database"
	"hitkeep/internal/worker"
	json "hitkeep/jsonapi"
)

// RecoveryError signals a recovery command that has already written its
// operator-facing failure to the command's error stream.
type RecoveryError struct{ Code int }

func (*RecoveryError) Error() string { return "recovery command failed" }

func recoveryExit(code int) error {
	if code == 0 {
		return nil
	}
	return &RecoveryError{Code: code}
}

func recoveryFlagExit(err error) error {
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return recoveryExit(2)
}

type recoveryCommand struct {
	ctx    context.Context
	in     io.Reader
	out    io.Writer
	errOut io.Writer
	logger *slog.Logger
}

// Recover handles the "hitkeep recover <subcommand>" family of commands.
// These are offline recovery operations that require HitKeep to be stopped
// (DuckDB allows only one writer at a time).
func Recover(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, logger *slog.Logger) error {
	if logger == nil {
		panic("hitkeepcmd: logger is required")
	}
	if len(args) == 0 {
		fmt.Fprintln(errOut, recoverUsage)
		return recoveryExit(1)
	}

	r := recoveryCommand{ctx: ctx, in: in, out: out, errOut: errOut, logger: logger}
	switch args[0] {
	case "disable-2fa":
		return r.recoverDisable2FA(args[1:])
	case "restore-backup":
		return r.recoverRestoreBackup(args[1:])
	case "restore-database-bundle":
		return r.recoverRestoreDatabaseBundle(args[1:])
	case "rebuild-default-tenant":
		return r.recoverRebuildDefaultTenant(args[1:])
	case "import-archives":
		return r.recoverImportArchives(args[1:])
	default:
		//nolint:gosec // G705: writes to stderr, not an HTTP response; %q safely quotes the argument.
		fmt.Fprintf(errOut, "Unknown recover subcommand: %q\n\n%s\n", args[0], recoverUsage)
		return recoveryExit(1)
	}
}

const recoverUsage = `Usage: hitkeep recover <subcommand> [flags]

Subcommands:
  disable-2fa      Remove all 2FA methods (TOTP + passkeys) for a user.
                   Allows the user to log in with email/password again.
  restore-backup   Restore databases from a backup snapshot.
  restore-database-bundle
                   Restore the exact database and WAL files retained before
                   an automatic DuckDB recovery.
  rebuild-default-tenant
                   Recreate a missing default tenant database as an empty,
                   schema-migrated file when no backup exists. Analytics
                   history is NOT restored.
  import-archives  Import local retention archive parquet exports back into
                   a tenant database, for example after rebuild-default-tenant.
                   Idempotent; rows already present are not duplicated.

Flags for disable-2fa:
  -email string   User email address (required)
  -db    string   Path to hitkeep.db (default: same as server config)
  -yes            Skip interactive confirmation prompt

Flags for restore-backup:
  -from      string   Backup source path (required) — local dir or s3://
  -snapshot  string   Specific snapshot timestamp (default: latest local)
  -db        string   Target hitkeep.db path (default: same as server config)
  -data-path string   Target data directory (default: same as server config)
  -yes                Skip confirmation prompt
  -s3-access-key-id, -s3-secret-access-key, -s3-session-token, -s3-region, -s3-endpoint,
  -s3-url-style, -s3-use-ssl   (fall back to HITKEEP_S3_* env vars)

Flags for restore-database-bundle:
  -from string   Recovery bundle directory containing manifest.json (required)
  -db   string   Target hitkeep.db path (default: same as server config)
  -yes           Skip interactive confirmation prompt

Flags for rebuild-default-tenant:
  -db        string   Path to hitkeep.db (default: same as server config)
  -data-path string   Tenant data directory (default: same as server config)
  -yes                Skip interactive confirmation prompt

Flags for import-archives:
  -db           string   Path to hitkeep.db (default: same as server config)
  -data-path    string   Tenant data directory (default: same as server config)
  -archive-path string   Local retention archive directory (default: same as server config)
  -tenant       string   Tenant ID to import into (default: the default tenant)
  -yes                   Skip interactive confirmation prompt

NOTE: HitKeep must be stopped before running recovery commands.
      DuckDB does not allow concurrent write access.`

type databaseRecoveryBundleManifest struct {
	Version   int                              `json:"version"`
	Artifacts []databaseRecoveryBundleArtifact `json:"artifacts"`
}

type databaseRecoveryBundleArtifact struct {
	Name         string `json:"name"`
	OriginalSize int64  `json:"original_size"`
	SHA256       string `json:"sha256"`
}

func (r recoveryCommand) recoverDisable2FA(args []string) error {
	fs := flag.NewFlagSet("disable-2fa", flag.ContinueOnError)
	fs.SetOutput(r.errOut)
	email := fs.String("email", "", "User email address (required)")
	dbPath := fs.String("db", "", "Path to hitkeep.db (defaults to server config value)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return recoveryFlagExit(err)
	}

	if *email == "" {
		fmt.Fprintln(r.errOut, "Error: -email is required")
		fmt.Fprintln(r.errOut)
		fs.Usage()
		return recoveryExit(1)
	}

	// Resolve DB path: flag overrides config default
	if *dbPath == "" {
		conf := config.Load(r.logger)
		*dbPath = conf.DBPath
	}

	ctx := r.ctx

	// ---- Connect --------------------------------------------------------
	fmt.Fprintf(r.out, "HitKeep Recovery — Disable 2FA\n")
	fmt.Fprintf(r.out, "================================\n")
	fmt.Fprintf(r.out, "DB:    %s\n", *dbPath)
	fmt.Fprintf(r.out, "User:  %s\n\n", *email)

	store, err := database.OpenMigratedStore(ctx, *dbPath)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: could not open database: %v\n", err)
		fmt.Fprintln(r.errOut, "Make sure HitKeep is stopped before running recovery commands.")
		return recoveryExit(1)
	}
	defer store.Close()

	// ---- Look up user ---------------------------------------------------
	user, err := store.GetUserByEmail(ctx, *email)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: database lookup failed: %v\n", err)
		return recoveryExit(1)
	}
	if user == nil {
		fmt.Fprintf(r.errOut, "Error: no user found with email %q\n", *email)
		return recoveryExit(1)
	}

	name := strings.TrimSpace(user.GivenName + " " + user.LastName)
	if name == "" {
		name = "(no name set)"
	}
	fmt.Fprintf(r.out, "Found user: %s (%s)\n\n", name, user.Email)

	// ---- Inventory active 2FA ------------------------------------------
	hasTOTP, err := store.HasEnabledTOTP(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: could not check TOTP status: %v\n", err)
		return recoveryExit(1)
	}

	passkeys, err := store.ListUserPasskeys(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: could not list passkeys: %v\n", err)
		return recoveryExit(1)
	}
	recoveryCodesRemaining, err := store.CountActiveRecoveryCodes(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: could not count recovery codes: %v\n", err)
		return recoveryExit(1)
	}

	if !hasTOTP && len(passkeys) == 0 && recoveryCodesRemaining == 0 {
		fmt.Fprintln(r.out, "No 2FA methods are active for this user. Nothing to do.")
		return nil
	}

	fmt.Fprintln(r.out, "Active 2FA methods:")
	if hasTOTP {
		fmt.Fprintln(r.out, "  TOTP:     enabled")
	} else {
		fmt.Fprintln(r.out, "  TOTP:     not enabled")
	}
	if len(passkeys) > 0 {
		fmt.Fprintf(r.out, "  Passkeys: %d\n", len(passkeys))
		for _, pk := range passkeys {
			fmt.Fprintf(r.out, "    - %s\n", pk.Name)
		}
	} else {
		fmt.Fprintln(r.out, "  Passkeys: none")
	}
	if recoveryCodesRemaining > 0 {
		fmt.Fprintf(r.out, "  Recovery codes: %d active\n", recoveryCodesRemaining)
	} else {
		fmt.Fprintln(r.out, "  Recovery codes: none")
	}

	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, "This will:")
	if hasTOTP {
		fmt.Fprintln(r.out, "  • Disable TOTP authenticator")
	}
	if len(passkeys) > 0 {
		fmt.Fprintf(r.out, "  • Delete %d passkey(s)\n", len(passkeys))
	}
	if recoveryCodesRemaining > 0 {
		fmt.Fprintf(r.out, "  • Invalidate %d recovery code(s)\n", recoveryCodesRemaining)
	}
	fmt.Fprintln(r.out, "  • Invalidate all remember-me sessions")
	fmt.Fprintln(r.out)

	// ---- Confirm -------------------------------------------------------
	if !*yes {
		fmt.Fprint(r.out, `Type "yes" to confirm: `)
		scanner := bufio.NewScanner(r.in)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer != "yes" {
			fmt.Fprintln(r.out, "Aborted.")
			return nil
		}
		fmt.Fprintln(r.out)
	}

	// ---- Execute -------------------------------------------------------
	result, err := store.DisableUserMFA(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: could not disable MFA: %v\n", err)
		return recoveryExit(1)
	}

	if result.TOTPDisabled {
		fmt.Fprintln(r.out, "✓ TOTP disabled")
	}
	if result.PasskeysDeleted > 0 {
		fmt.Fprintf(r.out, "✓ Deleted %d passkey(s)\n", result.PasskeysDeleted)
	}
	if recoveryCodesRemaining > 0 {
		fmt.Fprintf(r.out, "✓ Invalidated %d recovery code(s)\n", recoveryCodesRemaining)
	}
	if result.SessionsInvalidated > 0 {
		fmt.Fprintf(r.out, "✓ Invalidated %d remember-me session(s)\n", result.SessionsInvalidated)
	} else {
		fmt.Fprintln(r.out, "✓ Remember-me sessions invalidated")
	}

	fmt.Fprintf(r.out, "\nDone. %s can now log in with email and password.\n", user.Email)
	return nil
}

func (r recoveryCommand) recoverRestoreDatabaseBundle(args []string) error {
	fs := flag.NewFlagSet("restore-database-bundle", flag.ContinueOnError)
	fs.SetOutput(r.errOut)
	from := fs.String("from", "", "Recovery bundle directory containing manifest.json (required)")
	dbPath := fs.String("db", "", "Target hitkeep.db path (defaults to server config value)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return recoveryFlagExit(err)
	}
	if strings.TrimSpace(*from) == "" {
		fmt.Fprintln(r.errOut, "Error: -from is required")
		fs.Usage()
		return recoveryExit(1)
	}
	if strings.TrimSpace(*dbPath) == "" {
		*dbPath = config.Load(r.logger).DBPath
	}

	manifest, err := readDatabaseRecoveryBundle(*from)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: invalid recovery bundle: %v\n", err)
		return recoveryExit(1)
	}
	fmt.Fprintln(r.out, "HitKeep Recovery — Restore Database Bundle")
	fmt.Fprintln(r.out, "==========================================")
	fmt.Fprintf(r.out, "Source:    %s\n", *from)
	fmt.Fprintf(r.out, "Target DB: %s\n", *dbPath)
	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, "This restores the exact pre-recovery database and WAL state.")
	fmt.Fprintln(r.out, "It may intentionally restore the original DuckDB failure for rollback or forensic work.")
	fmt.Fprintln(r.out)
	if !*yes {
		fmt.Fprint(r.out, `Type "yes" to confirm: `)
		scanner := bufio.NewScanner(r.in)
		scanner.Scan()
		if strings.TrimSpace(scanner.Text()) != "yes" {
			fmt.Fprintln(r.out, "Aborted.")
			return nil
		}
	}

	if err := restoreDatabaseRecoveryBundle(*from, *dbPath, manifest); err != nil {
		fmt.Fprintf(r.errOut, "Error restoring database recovery bundle: %v\n", err)
		return recoveryExit(1)
	}
	fmt.Fprintln(r.out, "Database recovery bundle restored successfully.")
	return nil
}

func readDatabaseRecoveryBundle(bundleDir string) (databaseRecoveryBundleManifest, error) {
	data, err := os.ReadFile(filepath.Join(bundleDir, "manifest.json"))
	if err != nil {
		return databaseRecoveryBundleManifest{}, err
	}
	var manifest databaseRecoveryBundleManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return databaseRecoveryBundleManifest{}, err
	}
	if manifest.Version != 1 {
		return databaseRecoveryBundleManifest{}, fmt.Errorf("unsupported manifest version %d", manifest.Version)
	}
	if len(manifest.Artifacts) == 0 {
		return databaseRecoveryBundleManifest{}, fmt.Errorf("manifest has no artifacts")
	}
	hasDatabase := false
	for _, artifact := range manifest.Artifacts {
		switch artifact.Name {
		case "database.zst":
			hasDatabase = true
		case "wal.zst":
		default:
			return databaseRecoveryBundleManifest{}, fmt.Errorf("unsupported artifact %q", artifact.Name)
		}
		if artifact.OriginalSize < 0 || len(strings.TrimSpace(artifact.SHA256)) != sha256.Size*2 {
			return databaseRecoveryBundleManifest{}, fmt.Errorf("invalid metadata for artifact %q", artifact.Name)
		}
	}
	if !hasDatabase {
		return databaseRecoveryBundleManifest{}, fmt.Errorf("manifest has no database artifact")
	}
	return manifest, nil
}

func restoreDatabaseRecoveryBundle(bundleDir, targetPath string, manifest databaseRecoveryBundleManifest) error {
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}
	tempBase := filepath.Join(targetDir, fmt.Sprintf(".%s.bundle-restore-%d.tmp", filepath.Base(targetPath), time.Now().UTC().UnixNano()))
	tempWal := tempBase + ".wal"
	defer func() {
		_ = os.Remove(tempBase)
		_ = os.Remove(tempWal)
	}()

	for _, artifact := range manifest.Artifacts {
		target := tempBase
		if artifact.Name == "wal.zst" {
			target = tempWal
		}
		if err := decompressRecoveryBundleArtifact(
			filepath.Join(bundleDir, artifact.Name),
			target,
			artifact,
		); err != nil {
			return err
		}
	}

	backupPath, err := moveExistingDatabaseAside(targetPath)
	if err != nil {
		return err
	}
	return activateRestoredDatabase(tempBase, tempWal, targetPath, backupPath)
}

func decompressRecoveryBundleArtifact(sourcePath, targetPath string, artifact databaseRecoveryBundleArtifact) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open recovery artifact %q: %w", artifact.Name, err)
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create restored artifact %q: %w", artifact.Name, err)
	}
	defer target.Close()
	decoder, err := zstd.NewReader(source, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
	if err != nil {
		return fmt.Errorf("create recovery artifact decoder %q: %w", artifact.Name, err)
	}
	defer decoder.Close()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(target, hash), decoder)
	if err != nil {
		return fmt.Errorf("decompress recovery artifact %q: %w", artifact.Name, err)
	}
	if written != artifact.OriginalSize {
		return fmt.Errorf("recovery artifact %q size mismatch", artifact.Name)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(actual, artifact.SHA256) {
		return fmt.Errorf("recovery artifact %q checksum mismatch", artifact.Name)
	}
	if err := target.Sync(); err != nil {
		return fmt.Errorf("sync restored artifact %q: %w", artifact.Name, err)
	}
	return nil
}

func restoreMovedDatabaseBackup(targetPath, backupPath string) error {
	if backupPath == "" {
		return nil
	}
	var restoreErrors []error
	if fileExistsForRestore(backupPath) {
		if err := os.Rename(backupPath, targetPath); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore database: %w", err))
		}
	}
	if fileExistsForRestore(backupPath + ".wal") {
		if err := os.Rename(backupPath+".wal", targetPath+".wal"); err != nil {
			restoreErrors = append(restoreErrors, fmt.Errorf("restore WAL: %w", err))
		}
	}
	return errors.Join(restoreErrors...)
}

func activateRestoredDatabase(tempDatabasePath, tempWALPath, targetPath, backupPath string) (retErr error) {
	defer func() {
		if retErr == nil {
			return
		}
		_ = os.Remove(targetPath)
		_ = os.Remove(targetPath + ".wal")
		if restoreErr := restoreMovedDatabaseBackup(targetPath, backupPath); restoreErr != nil {
			retErr = fmt.Errorf("%w (failed to restore original database: %v)", retErr, restoreErr)
		}
	}()

	if err := os.Rename(tempDatabasePath, targetPath); err != nil {
		return fmt.Errorf("activate restored database: %w", err)
	}
	if fileExistsForRestore(tempWALPath) {
		if err := os.Rename(tempWALPath, targetPath+".wal"); err != nil {
			return fmt.Errorf("activate restored WAL: %w", err)
		}
	}
	if err := os.Chmod(targetPath, 0o600); err != nil {
		return fmt.Errorf("secure restored database: %w", err)
	}
	if fileExistsForRestore(targetPath + ".wal") {
		if err := os.Chmod(targetPath+".wal", 0o600); err != nil {
			return fmt.Errorf("secure restored WAL: %w", err)
		}
	}
	return nil
}

func fileExistsForRestore(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (r recoveryCommand) recoverRestoreBackup(args []string) error {
	fs := flag.NewFlagSet("restore-backup", flag.ContinueOnError)
	fs.SetOutput(r.errOut)
	from := fs.String("from", "", "Backup source path (required) — local dir or s3://")
	snapshot := fs.String("snapshot", "", "Specific snapshot timestamp (default: latest)")
	dbPath := fs.String("db", "", "Target hitkeep.db path (defaults to server config value)")
	dataPath := fs.String("data-path", "", "Target data directory (defaults to server config value)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")

	s3AccessKeyID := fs.String("s3-access-key-id", "", "S3 access key ID")
	s3SecretAccessKey := fs.String("s3-secret-access-key", "", "S3 secret access key")
	s3SessionToken := fs.String("s3-session-token", "", "S3 session token")
	s3Region := fs.String("s3-region", "", "S3 region")
	s3Endpoint := fs.String("s3-endpoint", "", "S3 custom endpoint")
	s3URLStyle := fs.String("s3-url-style", "", "S3 URL style: path or vhost")
	s3UseSSL := fs.Bool("s3-use-ssl", true, "S3 use SSL")

	if err := fs.Parse(args); err != nil {
		return recoveryFlagExit(err)
	}
	s3UseSSLSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "s3-use-ssl" {
			s3UseSSLSet = true
		}
	})

	if *from == "" {
		fmt.Fprintln(r.errOut, "Error: -from is required")
		fmt.Fprintln(r.errOut)
		fs.Usage()
		return recoveryExit(1)
	}

	// Resolve defaults from config.
	conf := config.Load(r.logger)
	if *dbPath == "" {
		*dbPath = conf.DBPath
	}
	if *dataPath == "" {
		*dataPath = conf.DataPath
	}

	isS3Source := worker.IsS3ArchivePath(*from)

	// Find snapshot.
	snapshotName := *snapshot
	if snapshotName == "" {
		if isS3Source {
			fmt.Fprintln(r.errOut, "Error: -snapshot is required when restoring from S3 (directory listing not supported)")
			return recoveryExit(1)
		}
		var err error
		snapshotName, err = findLatestLocalSnapshot(filepath.Join(*from, "shared"))
		if err != nil {
			fmt.Fprintf(r.errOut, "Error: could not find latest snapshot: %v\n", err)
			return recoveryExit(1)
		}
	}

	// Discover tenant backups.
	var tenantIDs []string
	if !isS3Source {
		var err error
		tenantIDs, err = discoverLocalTenantBackups(*from, snapshotName)
		if err != nil {
			r.logger.Warn("Could not discover tenant backups", "error", err)
		}
	}

	// Print summary.
	fmt.Fprintf(r.out, "HitKeep Recovery — Restore Backup\n")
	fmt.Fprintf(r.out, "====================================\n")
	fmt.Fprintf(r.out, "Source:     %s\n", *from)
	fmt.Fprintf(r.out, "Snapshot:   %s\n", snapshotName)
	fmt.Fprintf(r.out, "Target DB:  %s\n", *dbPath)
	fmt.Fprintf(r.out, "Data Path:  %s\n", *dataPath)
	if len(tenantIDs) > 0 {
		fmt.Fprintf(r.out, "Tenants:    %d tenant database(s)\n", len(tenantIDs))
		for _, id := range tenantIDs {
			fmt.Fprintf(r.out, "  - %s\n", id)
		}
	} else if isS3Source {
		fmt.Fprintln(r.out, "Tenants:    discovered from the restored control snapshot")
	}
	fmt.Fprintln(r.out)

	// Confirm.
	if !*yes {
		fmt.Fprint(r.out, `Type "yes" to confirm restore: `)
		scanner := bufio.NewScanner(r.in)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer != "yes" {
			fmt.Fprintln(r.out, "Aborted.")
			return nil
		}
		fmt.Fprintln(r.out)
	}

	ctx := r.ctx
	exitCode := 0

	// Build S3 config if source is S3.
	var s3Conf *worker.S3Config
	if isS3Source {
		s3Conf = resolveRestoreS3Config(conf, restoreS3Options{
			accessKeyID:     *s3AccessKeyID,
			secretAccessKey: *s3SecretAccessKey,
			sessionToken:    *s3SessionToken,
			region:          *s3Region,
			endpoint:        *s3Endpoint,
			urlStyle:        *s3URLStyle,
			useSSL:          *s3UseSSL,
			useSSLSet:       s3UseSSLSet,
		})
	}

	// Restore shared DB.
	sharedSource := joinRestorePath(*from, "shared", snapshotName)
	sharedRestored := false
	if err := restoreDatabase(ctx, r.out, r.logger, *dbPath, sharedSource, isS3Source, s3Conf); err != nil {
		fmt.Fprintf(r.errOut, "Error restoring shared database: %v\n", err)
		exitCode = 1
	} else {
		sharedRestored = true
		fmt.Fprintf(r.out, "Shared database restored to %s\n", *dbPath)
	}
	if isS3Source && sharedRestored {
		var err error
		tenantIDs, err = discoverS3TenantBackupsFromControl(ctx, r.logger, *dbPath)
		if err != nil {
			fmt.Fprintf(r.errOut, "Error discovering tenant backups from restored control database: %v\n", err)
			exitCode = 1
			tenantIDs = nil
		} else {
			fmt.Fprintf(r.out, "Discovered %d tenant database(s) from restored control snapshot.\n", len(tenantIDs))
		}
	}

	// Restore tenant DBs.
	for _, tenantID := range tenantIDs {
		tenantDir := filepath.Join(*dataPath, "tenants", tenantID)
		if err := os.MkdirAll(tenantDir, 0755); err != nil {
			fmt.Fprintf(r.errOut, "Error creating tenant directory %s: %v\n", tenantDir, err)
			exitCode = 1
			continue
		}

		tenantDBPath := filepath.Join(tenantDir, "hitkeep.db")
		tenantSource := joinRestorePath(*from, "tenants", tenantID, snapshotName)
		if err := restoreDatabase(ctx, r.out, r.logger, tenantDBPath, tenantSource, isS3Source, s3Conf); err != nil {
			fmt.Fprintf(r.errOut, "Error restoring tenant %s: %v\n", tenantID, err)
			exitCode = 1
		} else {
			fmt.Fprintf(r.out, "Tenant %s restored to %s\n", tenantID, tenantDBPath)
		}
	}

	if exitCode == 0 {
		fmt.Fprintln(r.out, "\nRestore completed successfully.")
	} else {
		fmt.Fprintln(r.errOut, "\nRestore completed with errors (see above).")
	}
	return recoveryExit(exitCode)
}

// restoreDatabase imports a backup snapshot into a fresh DuckDB at targetPath.
// If targetPath already exists, it is renamed as a safety net.
func restoreDatabase(ctx context.Context, out io.Writer, logger *slog.Logger, targetPath, sourcePath string, isS3 bool, s3Conf *worker.S3Config) error {
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("create target directory %s: %w", targetDir, err)
	}

	tempPath := filepath.Join(targetDir, fmt.Sprintf(".%s.restore-%d.tmp", filepath.Base(targetPath), time.Now().UTC().UnixNano()))
	tempWalPath := tempPath + ".wal"
	defer func() {
		_ = os.Remove(tempWalPath)
		_ = os.Remove(tempPath)
	}()

	// Import into a temporary database first so the final restored DB does not
	// depend on a WAL created by the recovery command itself.
	store := database.NewStore(tempPath, database.WithLogger(logger))
	if err := store.Connect(); err != nil {
		return fmt.Errorf("could not create target database: %w", err)
	}
	defer func() { _ = store.Close() }()

	db := store.DB()
	if err := database.WithDuckDBSession(ctx, db, database.DuckDBSessionOptions{
		S3: s3ConfigForRestore(isS3, s3Conf),
	}, func(conn *sql.Conn) error {
		safePath := strings.ReplaceAll(sourcePath, "'", "''")
		query := fmt.Sprintf("IMPORT DATABASE '%s';", safePath)
		if _, err := conn.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("import database from %s: %w", sourcePath, err)
		}
		if _, err := conn.ExecContext(ctx, "CHECKPOINT;"); err != nil {
			return fmt.Errorf("checkpoint restored database: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := finalizeRestoredDatabase(tempPath, store); err != nil {
		return err
	}

	backupPath, err := moveExistingDatabaseAside(targetPath)
	if err != nil {
		return err
	}
	if backupPath != "" {
		fmt.Fprintf(out, "  Existing DB renamed to %s\n", backupPath)
	}
	return activateRestoredDatabase(tempPath, tempWalPath, targetPath, backupPath)
}

func s3ConfigForRestore(enabled bool, cfg *worker.S3Config) *database.S3SecretConfig {
	if !enabled {
		return nil
	}
	return cfg
}

type restoreS3Options struct {
	accessKeyID     string
	secretAccessKey string
	sessionToken    string
	region          string
	endpoint        string
	urlStyle        string
	useSSL          bool
	useSSLSet       bool
}

func resolveRestoreS3Config(conf *config.Config, opts restoreS3Options) *worker.S3Config {
	if opts.accessKeyID == "" {
		opts.accessKeyID = conf.S3AccessKeyID
	}
	if opts.secretAccessKey == "" {
		opts.secretAccessKey = conf.S3SecretAccessKey
	}
	if opts.sessionToken == "" {
		opts.sessionToken = conf.S3SessionToken
	}
	if opts.region == "" {
		opts.region = conf.S3Region
	}
	if opts.endpoint == "" {
		opts.endpoint = conf.S3Endpoint
	}
	if opts.urlStyle == "" {
		opts.urlStyle = conf.S3URLStyle
	}
	if !opts.useSSLSet {
		opts.useSSL = conf.S3UseSSL
	}
	return &worker.S3Config{
		AccessKeyID:     opts.accessKeyID,
		SecretAccessKey: opts.secretAccessKey,
		SessionToken:    opts.sessionToken,
		Region:          opts.region,
		Endpoint:        opts.endpoint,
		URLStyle:        opts.urlStyle,
		UseSSL:          opts.useSSL,
	}
}

func finalizeRestoredDatabase(dbPath string, store *database.Store) error {
	if err := store.Close(); err != nil {
		return fmt.Errorf("close restored database: %w", err)
	}

	walPath := dbPath + ".wal"
	if _, err := os.Stat(walPath); err == nil {
		return fmt.Errorf("restored database left unexpected WAL file %s; aborting to avoid partially replayable state", walPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat restored WAL %s: %w", walPath, err)
	}

	return nil
}

func moveExistingDatabaseAside(targetPath string) (string, error) {
	backup := fmt.Sprintf("%s.pre-restore.%s", targetPath, time.Now().UTC().Format("2006-01-02T150405.000000000Z"))
	databaseRenamed := false
	walRenamed := false

	if _, err := os.Stat(targetPath); err == nil {
		if err := os.Rename(targetPath, backup); err != nil {
			return "", fmt.Errorf("could not rename existing database %s to %s: %w", targetPath, backup, err)
		}
		databaseRenamed = true
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat existing database %s: %w", targetPath, err)
	}

	walPath := targetPath + ".wal"
	if _, err := os.Stat(walPath); err == nil {
		walBackup := backup + ".wal"
		if renameErr := os.Rename(walPath, walBackup); renameErr != nil {
			rollbackErr := restoreMovedDatabaseBackup(targetPath, backup)
			if rollbackErr != nil {
				return "", fmt.Errorf(
					"could not rename existing WAL %s to %s: %w (failed to restore database: %v)",
					walPath,
					walBackup,
					renameErr,
					rollbackErr,
				)
			}
			return "", fmt.Errorf("could not rename existing WAL %s to %s: %w", walPath, walBackup, renameErr)
		}
		walRenamed = true
	} else if err != nil && !os.IsNotExist(err) {
		if databaseRenamed {
			if rollbackErr := restoreMovedDatabaseBackup(targetPath, backup); rollbackErr != nil {
				return "", fmt.Errorf("stat existing WAL %s: %w (failed to restore database: %v)", walPath, err, rollbackErr)
			}
		}
		return "", fmt.Errorf("stat existing WAL %s: %w", walPath, err)
	}

	if databaseRenamed || walRenamed {
		return backup, nil
	}

	return "", nil
}

// findLatestLocalSnapshot finds the latest snapshot directory (lexicographic sort)
// under the given directory.
func findLatestLocalSnapshot(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("could not read snapshot directory %s: %w", dir, err)
	}

	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}

	if len(dirs) == 0 {
		return "", fmt.Errorf("no snapshots found in %s", dir)
	}

	sort.Strings(dirs)
	return dirs[len(dirs)-1], nil
}

// discoverLocalTenantBackups returns tenant ID directory names that have a
// matching snapshot subdirectory under {from}/tenants/.
func discoverLocalTenantBackups(fromPath, snapshotName string) ([]string, error) {
	tenantsDir := filepath.Join(fromPath, "tenants")
	entries, err := os.ReadDir(tenantsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("could not read tenants directory: %w", err)
	}

	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Check that this tenant has the requested snapshot.
		snapshotDir := filepath.Join(tenantsDir, e.Name(), snapshotName)
		if info, err := os.Stat(snapshotDir); err == nil && info.IsDir() {
			ids = append(ids, e.Name())
		}
	}

	return ids, nil
}

// discoverS3TenantBackupsFromControl derives S3 object prefixes from the
// restored control database because restore-backup intentionally does not list
// buckets. Split snapshots contain every active tenant, including the default;
// legacy snapshots contain only non-default tenant exports because the default
// tenant still lived in the shared database.
func discoverS3TenantBackupsFromControl(ctx context.Context, logger *slog.Logger, controlPath string) ([]string, error) {
	control := database.NewStore(controlPath, database.WithLogger(logger))
	if err := control.Connect(); err != nil {
		return nil, fmt.Errorf("open restored control database: %w", err)
	}
	defer control.Close()

	split, err := control.HasDefaultTenantSplit(ctx)
	if err != nil {
		return nil, err
	}
	var tenantIDs []uuid.UUID
	if split {
		tenantIDs, err = control.ListActiveTenantIDs(ctx)
	} else {
		tenantIDs, err = control.ListNonDefaultTenantIDs(ctx)
	}
	if err != nil {
		if isMissingTenantBackupSchema(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list tenants in restored control database: %w", err)
	}

	ids := make([]string, 0, len(tenantIDs))
	for _, tenantID := range tenantIDs {
		ids = append(ids, tenantID.String())
	}
	return ids, nil
}

func isMissingTenantBackupSchema(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "tenants") &&
		(strings.Contains(message, "does not exist") || strings.Contains(message, "not found"))
}

// joinRestorePath joins path segments, handling both local and S3 paths.
func joinRestorePath(parts ...string) string {
	if len(parts) == 0 {
		return ""
	}
	if worker.IsS3ArchivePath(parts[0]) {
		var normalized strings.Builder
		normalized.WriteString(strings.TrimRight(parts[0], "/"))
		for _, p := range parts[1:] {
			clean := strings.Trim(p, "/")
			if clean != "" {
				normalized.WriteString("/" + clean)
			}
		}
		return normalized.String()
	}
	return filepath.Join(parts...)
}

func (r recoveryCommand) recoverRebuildDefaultTenant(args []string) error {
	fs := flag.NewFlagSet("rebuild-default-tenant", flag.ContinueOnError)
	fs.SetOutput(r.errOut)
	dbPath := fs.String("db", "", "Path to hitkeep.db (defaults to server config value)")
	dataPath := fs.String("data-path", "", "Tenant data directory (defaults to server config value)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return recoveryFlagExit(err)
	}
	conf := config.Load(r.logger)
	if *dbPath == "" {
		*dbPath = conf.DBPath
	}
	if *dataPath == "" {
		*dataPath = conf.DataPath
	}

	fmt.Fprintf(r.out, "HitKeep Recovery — Rebuild Default Tenant Database\n")
	fmt.Fprintf(r.out, "===================================================\n")
	fmt.Fprintf(r.out, "Control DB: %s\n", *dbPath)
	fmt.Fprintf(r.out, "Data path:  %s\n\n", *dataPath)
	fmt.Fprintln(r.out, "This recreates the default tenant's database as an EMPTY, schema-migrated")
	fmt.Fprintln(r.out, "file with fresh site mirrors. Its analytics history is NOT restored; use a")
	fmt.Fprintln(r.out, "backup restore instead whenever one exists. HitKeep must be stopped.")
	fmt.Fprintln(r.out)

	if !*yes {
		fmt.Fprint(r.out, `Type "yes" to confirm: `)
		scanner := bufio.NewScanner(r.in)
		scanner.Scan()
		if strings.TrimSpace(scanner.Text()) != "yes" {
			fmt.Fprintln(r.out, "Aborted.")
			return nil
		}
		fmt.Fprintln(r.out)
	}

	tenantPath, err := database.RebuildDefaultTenantFile(r.ctx, *dbPath, *dataPath,
		database.WithLogger(r.logger),
		database.WithMemoryLimit(conf.DuckDBMemoryLimit),
		database.WithThreads(conf.DuckDBThreads),
	)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: %v\n", err)
		fmt.Fprintln(r.errOut, "Make sure HitKeep is stopped before running recovery commands.")
		return recoveryExit(1)
	}
	fmt.Fprintf(r.out, "✓ Rebuilt empty default tenant database at %s\n", tenantPath)
	fmt.Fprintln(r.out, "Start HitKeep to begin collecting analytics again.")
	return nil
}

func (r recoveryCommand) recoverImportArchives(args []string) error {
	fs := flag.NewFlagSet("import-archives", flag.ContinueOnError)
	fs.SetOutput(r.errOut)
	dbPath := fs.String("db", "", "Path to hitkeep.db (defaults to server config value)")
	dataPath := fs.String("data-path", "", "Tenant data directory (defaults to server config value)")
	archivePath := fs.String("archive-path", "", "Local retention archive directory (defaults to server config value)")
	tenantFlag := fs.String("tenant", "", "Tenant ID to import into (defaults to the default tenant)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return recoveryFlagExit(err)
	}
	conf := config.Load(r.logger)
	if *dbPath == "" {
		*dbPath = conf.DBPath
	}
	if *dataPath == "" {
		*dataPath = conf.DataPath
	}
	if *archivePath == "" {
		*archivePath = conf.ArchivePath
	}
	if worker.IsS3ArchivePath(*archivePath) {
		fmt.Fprintln(r.errOut, "Error: import-archives reads local archives only; download the s3:// archive directory first and pass it via -archive-path")
		return recoveryExit(1)
	}

	ctx := r.ctx
	control, err := database.OpenMigratedStore(ctx, *dbPath)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: could not open database: %v\n", err)
		fmt.Fprintln(r.errOut, "Make sure HitKeep is stopped before running recovery commands.")
		return recoveryExit(1)
	}
	defaultID, err := control.GetDefaultTenantID(ctx)
	if closeErr := control.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: could not resolve default tenant: %v\n", err)
		return recoveryExit(1)
	}
	tenantID := defaultID
	if *tenantFlag != "" {
		tenantID, err = uuid.Parse(*tenantFlag)
		if err != nil {
			fmt.Fprintf(r.errOut, "Error: invalid -tenant %q: %v\n", *tenantFlag, err)
			return recoveryExit(1)
		}
	}

	files, err := database.DiscoverLocalRetentionArchives(*archivePath, tenantID, tenantID == defaultID)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: %v\n", err)
		return recoveryExit(1)
	}
	if len(files) == 0 {
		fmt.Fprintf(r.out, "No retention archives found for tenant %s under %s. Nothing to do.\n", tenantID, *archivePath)
		return nil
	}
	tenantPath := filepath.Join(*dataPath, "tenants", tenantID.String(), "hitkeep.db")

	fmt.Fprintf(r.out, "HitKeep Recovery — Import Retention Archives\n")
	fmt.Fprintf(r.out, "=============================================\n")
	fmt.Fprintf(r.out, "Tenant DB: %s\n", tenantPath)
	fmt.Fprintf(r.out, "Archives:  %d parquet file(s) under %s\n\n", len(files), *archivePath)
	fmt.Fprintln(r.out, "Rows already present are kept once; rows whose parent row no longer")
	fmt.Fprintln(r.out, "exists are skipped. HitKeep must be stopped.")
	fmt.Fprintln(r.out)

	if !*yes {
		fmt.Fprint(r.out, `Type "yes" to confirm: `)
		scanner := bufio.NewScanner(r.in)
		scanner.Scan()
		if strings.TrimSpace(scanner.Text()) != "yes" {
			fmt.Fprintln(r.out, "Aborted.")
			return nil
		}
		fmt.Fprintln(r.out)
	}

	summary, err := database.ImportRetentionArchives(ctx, tenantPath, files,
		database.WithMemoryLimit(conf.DuckDBMemoryLimit),
		database.WithThreads(conf.DuckDBThreads),
	)
	if err != nil {
		fmt.Fprintf(r.errOut, "Error: %v\n", err)
		return recoveryExit(1)
	}

	tables := make([]string, 0, len(summary.Imported))
	for table := range summary.Imported {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	fmt.Fprintf(r.out, "✓ Imported %d archive file(s)\n", summary.Files)
	for _, table := range tables {
		if summary.Imported[table] == 0 && summary.Skipped[table] == 0 {
			continue
		}
		fmt.Fprintf(r.out, "  %-32s %8d imported %8d skipped\n", table, summary.Imported[table], summary.Skipped[table])
	}
	return nil
}
