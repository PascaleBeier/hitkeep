package hitkeepcmd

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"

	"hitkeep/internal/api"
	"hitkeep/internal/config"
	"hitkeep/internal/controlstore"
	"hitkeep/internal/database"
	"hitkeep/internal/worker"
)

// Recover handles the "hitkeep recover <subcommand>" family of commands.
// These are offline recovery operations that require HitKeep to be stopped
// (DuckDB allows only one writer at a time).
func Recover() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, recoverUsage)
		os.Exit(1)
	}

	switch os.Args[2] {
	case "disable-2fa":
		recoverDisable2FA(os.Args[3:])
	case "restore-backup":
		recoverRestoreBackup(os.Args[3:])
	case "restore-database-bundle":
		recoverRestoreDatabaseBundle(os.Args[3:])
	default:
		//nolint:gosec // G705: writes to stderr, not an HTTP response; %q safely quotes the argument.
		fmt.Fprintf(os.Stderr, "Unknown recover subcommand: %q\n\n%s\n", os.Args[2], recoverUsage)
		os.Exit(1)
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

type offlineMFAResult struct {
	TOTPDisabled        bool
	PasskeysDeleted     int
	SessionsInvalidated int
}

type offlineMFAStore struct {
	close                    func() error
	getUserByEmail           func(context.Context, string) (*api.User, error)
	hasEnabledTOTP           func(context.Context, uuid.UUID) (bool, error)
	listUserPasskeys         func(context.Context, uuid.UUID) ([]api.UserPasskey, error)
	countActiveRecoveryCodes func(context.Context, uuid.UUID) (int, error)
	disableUserMFA           func(context.Context, uuid.UUID) (offlineMFAResult, error)
}

func openOfflineMFAStore(ctx context.Context, path string) (offlineMFAStore, error) {
	format, err := controlstore.InspectFormat(path)
	if err != nil {
		return offlineMFAStore{}, err
	}
	if format == controlstore.FileSQLite {
		store, err := controlstore.Open(ctx, path)
		if err != nil {
			return offlineMFAStore{}, err
		}
		return offlineMFAStore{
			close:                    store.Close,
			getUserByEmail:           store.GetUserByEmail,
			hasEnabledTOTP:           store.HasEnabledTOTP,
			listUserPasskeys:         store.ListUserPasskeys,
			countActiveRecoveryCodes: store.CountActiveRecoveryCodes,
			disableUserMFA: func(ctx context.Context, userID uuid.UUID) (offlineMFAResult, error) {
				result, err := store.DisableUserMFA(ctx, userID)
				return offlineMFAResult{TOTPDisabled: result.TOTPDisabled, PasskeysDeleted: result.PasskeysDeleted, SessionsInvalidated: result.SessionsInvalidated}, err
			},
		}, nil
	}
	if format != controlstore.FileDuckDB {
		return offlineMFAStore{}, fmt.Errorf("control database has unsupported format %s", format)
	}
	store, err := database.OpenMigratedStore(ctx, path)
	if err != nil {
		return offlineMFAStore{}, err
	}
	return offlineMFAStore{
		close:                    store.Close,
		getUserByEmail:           store.GetUserByEmail,
		hasEnabledTOTP:           store.HasEnabledTOTP,
		listUserPasskeys:         store.ListUserPasskeys,
		countActiveRecoveryCodes: store.CountActiveRecoveryCodes,
		disableUserMFA: func(ctx context.Context, userID uuid.UUID) (offlineMFAResult, error) {
			result, err := store.DisableUserMFA(ctx, userID)
			return offlineMFAResult{TOTPDisabled: result.TOTPDisabled, PasskeysDeleted: result.PasskeysDeleted, SessionsInvalidated: result.SessionsInvalidated}, err
		},
	}, nil
}

func recoverDisable2FA(args []string) {
	fs := flag.NewFlagSet("disable-2fa", flag.ExitOnError)
	email := fs.String("email", "", "User email address (required)")
	dbPath := fs.String("db", "", "Path to hitkeep.db (defaults to server config value)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

	if *email == "" {
		fmt.Fprintln(os.Stderr, "Error: -email is required")
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		os.Exit(1)
	}

	// Resolve DB path: flag overrides config default
	if *dbPath == "" {
		conf := config.Load()
		*dbPath = conf.DBPath
	}

	ctx := context.Background()

	// ---- Connect --------------------------------------------------------
	fmt.Printf("HitKeep Recovery — Disable 2FA\n")
	fmt.Printf("================================\n")
	fmt.Printf("DB:    %s\n", *dbPath)
	fmt.Printf("User:  %s\n\n", *email)

	store, err := openOfflineMFAStore(ctx, *dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not open database: %v\n", err)
		fmt.Fprintln(os.Stderr, "Make sure HitKeep is stopped before running recovery commands.")
		os.Exit(1)
	}
	defer store.close()

	// ---- Look up user ---------------------------------------------------
	user, err := store.getUserByEmail(ctx, *email)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: database lookup failed: %v\n", err)
		os.Exit(1)
	}
	if user == nil {
		fmt.Fprintf(os.Stderr, "Error: no user found with email %q\n", *email)
		os.Exit(1)
		return
	}

	name := strings.TrimSpace(user.GivenName + " " + user.LastName)
	if name == "" {
		name = "(no name set)"
	}
	fmt.Printf("Found user: %s (%s)\n\n", name, user.Email)

	// ---- Inventory active 2FA ------------------------------------------
	hasTOTP, err := store.hasEnabledTOTP(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not check TOTP status: %v\n", err)
		os.Exit(1)
	}

	passkeys, err := store.listUserPasskeys(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not list passkeys: %v\n", err)
		os.Exit(1)
	}
	recoveryCodesRemaining, err := store.countActiveRecoveryCodes(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not count recovery codes: %v\n", err)
		os.Exit(1)
	}

	if !hasTOTP && len(passkeys) == 0 && recoveryCodesRemaining == 0 {
		fmt.Println("No 2FA methods are active for this user. Nothing to do.")
		os.Exit(0)
	}

	fmt.Println("Active 2FA methods:")
	if hasTOTP {
		fmt.Println("  TOTP:     enabled")
	} else {
		fmt.Println("  TOTP:     not enabled")
	}
	if len(passkeys) > 0 {
		fmt.Printf("  Passkeys: %d\n", len(passkeys))
		for _, pk := range passkeys {
			fmt.Printf("    - %s\n", pk.Name)
		}
	} else {
		fmt.Println("  Passkeys: none")
	}
	if recoveryCodesRemaining > 0 {
		fmt.Printf("  Recovery codes: %d active\n", recoveryCodesRemaining)
	} else {
		fmt.Println("  Recovery codes: none")
	}

	fmt.Println()
	fmt.Println("This will:")
	if hasTOTP {
		fmt.Println("  • Disable TOTP authenticator")
	}
	if len(passkeys) > 0 {
		fmt.Printf("  • Delete %d passkey(s)\n", len(passkeys))
	}
	if recoveryCodesRemaining > 0 {
		fmt.Printf("  • Invalidate %d recovery code(s)\n", recoveryCodesRemaining)
	}
	fmt.Println("  • Invalidate all remember-me sessions")
	fmt.Println()

	// ---- Confirm -------------------------------------------------------
	if !*yes {
		fmt.Print(`Type "yes" to confirm: `)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer != "yes" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
		fmt.Println()
	}

	// ---- Execute -------------------------------------------------------
	result, err := store.disableUserMFA(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not disable MFA: %v\n", err)
		os.Exit(1)
	}

	if result.TOTPDisabled {
		fmt.Println("✓ TOTP disabled")
	}
	if result.PasskeysDeleted > 0 {
		fmt.Printf("✓ Deleted %d passkey(s)\n", result.PasskeysDeleted)
	}
	if recoveryCodesRemaining > 0 {
		fmt.Printf("✓ Invalidated %d recovery code(s)\n", recoveryCodesRemaining)
	}
	if result.SessionsInvalidated > 0 {
		fmt.Printf("✓ Invalidated %d remember-me session(s)\n", result.SessionsInvalidated)
	} else {
		fmt.Println("✓ Remember-me sessions invalidated")
	}

	fmt.Printf("\nDone. %s can now log in with email and password.\n", user.Email)
	os.Exit(0)
}

func recoverRestoreDatabaseBundle(args []string) {
	fs := flag.NewFlagSet("restore-database-bundle", flag.ExitOnError)
	from := fs.String("from", "", "Recovery bundle directory containing manifest.json (required)")
	dbPath := fs.String("db", "", "Target hitkeep.db path (defaults to server config value)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}
	if strings.TrimSpace(*from) == "" {
		fmt.Fprintln(os.Stderr, "Error: -from is required")
		fs.Usage()
		os.Exit(1)
	}
	if strings.TrimSpace(*dbPath) == "" {
		*dbPath = config.Load().DBPath
	}

	manifest, err := readDatabaseRecoveryBundle(*from)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid recovery bundle: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("HitKeep Recovery — Restore Database Bundle")
	fmt.Println("==========================================")
	fmt.Printf("Source:    %s\n", *from)
	fmt.Printf("Target DB: %s\n", *dbPath)
	fmt.Println()
	fmt.Println("This restores the exact pre-recovery database and WAL state.")
	fmt.Println("It may intentionally restore the original DuckDB failure for rollback or forensic work.")
	fmt.Println()
	if !*yes {
		fmt.Print(`Type "yes" to confirm: `)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		if strings.TrimSpace(scanner.Text()) != "yes" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
	}

	if err := restoreDatabaseRecoveryBundle(*from, *dbPath, manifest); err != nil {
		fmt.Fprintf(os.Stderr, "Error restoring database recovery bundle: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Database recovery bundle restored successfully.")
	os.Exit(0)
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

func recoverRestoreBackup(args []string) {
	fs := flag.NewFlagSet("restore-backup", flag.ExitOnError)
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
		os.Exit(1)
	}
	s3UseSSLSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "s3-use-ssl" {
			s3UseSSLSet = true
		}
	})

	if *from == "" {
		fmt.Fprintln(os.Stderr, "Error: -from is required")
		fmt.Fprintln(os.Stderr)
		fs.Usage()
		os.Exit(1)
	}

	// Resolve defaults from config.
	conf := config.Load()
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
			fmt.Fprintln(os.Stderr, "Error: -snapshot is required when restoring from S3 (directory listing not supported)")
			os.Exit(1)
		}
		var err error
		snapshotName, err = findLatestLocalSnapshot(filepath.Join(*from, "shared"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: could not find latest snapshot: %v\n", err)
			os.Exit(1)
		}
	}

	// Discover tenant backups.
	var tenantIDs []string
	if !isS3Source {
		var err error
		tenantIDs, err = discoverLocalTenantBackups(*from, snapshotName)
		if err != nil {
			slog.Warn("Could not discover tenant backups", "error", err)
		}
	}

	// Print summary.
	fmt.Printf("HitKeep Recovery — Restore Backup\n")
	fmt.Printf("====================================\n")
	fmt.Printf("Source:     %s\n", *from)
	fmt.Printf("Snapshot:   %s\n", snapshotName)
	fmt.Printf("Target DB:  %s\n", *dbPath)
	fmt.Printf("Data Path:  %s\n", *dataPath)
	if len(tenantIDs) > 0 {
		fmt.Printf("Tenants:    %d tenant database(s)\n", len(tenantIDs))
		for _, id := range tenantIDs {
			fmt.Printf("  - %s\n", id)
		}
	} else if isS3Source {
		fmt.Println("Tenants:    discovered from the restored control snapshot")
	}
	fmt.Println()

	// Confirm.
	if !*yes {
		fmt.Print(`Type "yes" to confirm restore: `)
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Scan()
		answer := strings.TrimSpace(scanner.Text())
		if answer != "yes" {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
		fmt.Println()
	}

	ctx := context.Background()
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

	// Restore the control snapshot. New backups contain a compressed SQLite
	// control database; legacy backups remain DuckDB exports and are converted by
	// normal offline startup after restore.
	sharedSource := joinRestorePath(*from, "shared", snapshotName)
	sharedRestored := false
	restoreErr := restoreSQLiteControlDatabase(ctx, *dbPath, sharedSource, isS3Source, s3Conf)
	if errors.Is(restoreErr, errNoSQLiteControlSnapshot) {
		restoreErr = restoreDatabase(ctx, *dbPath, sharedSource, isS3Source, s3Conf)
	}
	if restoreErr != nil {
		fmt.Fprintf(os.Stderr, "Error restoring control database: %v\n", restoreErr)
		exitCode = 1
	} else {
		sharedRestored = true
		fmt.Printf("Control database restored to %s\n", *dbPath)
	}
	if sharedRestored {
		var err error
		tenantIDs, err = discoverS3TenantBackupsFromControl(ctx, *dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error discovering tenant backups from restored control database: %v\n", err)
			exitCode = 1
			tenantIDs = nil
		} else if !isS3Source {
			if err := validateLocalTenantSnapshots(*from, snapshotName, tenantIDs); err != nil {
				fmt.Fprintf(os.Stderr, "Error validating tenant backup set: %v\n", err)
				exitCode = 1
				tenantIDs = nil
			} else {
				fmt.Printf("Validated %d tenant database(s) from restored control snapshot.\n", len(tenantIDs))
			}
		} else {
			fmt.Printf("Discovered %d tenant database(s) from restored control snapshot.\n", len(tenantIDs))
		}
	}

	// Restore tenant DBs.
	for _, tenantID := range tenantIDs {
		tenantDir := filepath.Join(*dataPath, "tenants", tenantID)
		if err := os.MkdirAll(tenantDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating tenant directory %s: %v\n", tenantDir, err)
			exitCode = 1
			continue
		}

		tenantDBPath := filepath.Join(tenantDir, "hitkeep.db")
		tenantSource := joinRestorePath(*from, "tenants", tenantID, snapshotName)
		if err := restoreDatabase(ctx, tenantDBPath, tenantSource, isS3Source, s3Conf); err != nil {
			fmt.Fprintf(os.Stderr, "Error restoring tenant %s: %v\n", tenantID, err)
			exitCode = 1
		} else {
			fmt.Printf("Tenant %s restored to %s\n", tenantID, tenantDBPath)
		}
	}

	if exitCode == 0 {
		fmt.Println("\nRestore completed successfully.")
	} else {
		fmt.Fprintln(os.Stderr, "\nRestore completed with errors (see above).")
	}
	os.Exit(exitCode)
}

var errNoSQLiteControlSnapshot = errors.New("backup does not contain a SQLite control snapshot")

type sqliteControlBackupManifest struct {
	Version int    `json:"version"`
	Engine  string `json:"engine"`
	File    string `json:"file"`
	Bytes   int64  `json:"bytes"`
	SHA256  string `json:"sha256"`
}

func restoreSQLiteControlDatabase(ctx context.Context, targetPath, sourcePath string, isS3 bool, s3Conf *worker.S3Config) error {
	targetDir := filepath.Dir(targetPath)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create control restore directory: %w", err)
	}
	staging, err := os.MkdirTemp(targetDir, ".control-restore-*")
	if err != nil {
		return fmt.Errorf("create control restore staging directory: %w", err)
	}
	if err := os.Chmod(staging, 0o700); err != nil {
		_ = os.RemoveAll(staging)
		return err
	}
	defer os.RemoveAll(staging)

	manifestPath := filepath.Join(staging, "manifest.json")
	compressedPath := filepath.Join(staging, "control.db.zst")
	completePath := filepath.Join(staging, "_COMPLETE")
	if isS3 {
		for name, destination := range map[string]string{
			"_COMPLETE":      completePath,
			"manifest.json":  manifestPath,
			"control.db.zst": compressedPath,
		} {
			err := downloadRestoreS3Object(ctx, joinRestorePath(sourcePath, name), destination, s3Conf)
			if isS3ObjectMissing(err) {
				return errNoSQLiteControlSnapshot
			}
			if err != nil {
				return fmt.Errorf("download SQLite control backup %s: %w", name, err)
			}
		}
	} else {
		if _, err := os.Stat(filepath.Join(sourcePath, "control.db.zst")); errors.Is(err, os.ErrNotExist) {
			return errNoSQLiteControlSnapshot
		} else if err != nil {
			return fmt.Errorf("inspect SQLite control snapshot: %w", err)
		}
		if _, err := os.Stat(filepath.Join(sourcePath, "_COMPLETE")); err != nil {
			return fmt.Errorf("SQLite control snapshot is incomplete: %w", err)
		}
		for source, destination := range map[string]string{
			filepath.Join(sourcePath, "_COMPLETE"):      completePath,
			filepath.Join(sourcePath, "manifest.json"):  manifestPath,
			filepath.Join(sourcePath, "control.db.zst"): compressedPath,
		} {
			if err := copyRestoreFile(source, destination); err != nil {
				return err
			}
		}
	}
	if _, err := os.Stat(completePath); err != nil {
		return fmt.Errorf("SQLite control snapshot is incomplete: %w", err)
	}
	payload, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read SQLite control backup manifest: %w", err)
	}
	var manifest sqliteControlBackupManifest
	if err := json.Unmarshal(payload, &manifest); err != nil {
		return fmt.Errorf("decode SQLite control backup manifest: %w", err)
	}
	if manifest.Version != 1 || manifest.Engine != "sqlite" || manifest.File != "control.db.zst" || manifest.Bytes < 0 || len(manifest.SHA256) != 64 {
		return fmt.Errorf("unsupported or malformed SQLite control backup manifest")
	}

	tempPath := filepath.Join(targetDir, fmt.Sprintf(".%s.restore-%d.tmp", filepath.Base(targetPath), time.Now().UTC().UnixNano()))
	defer os.Remove(tempPath)
	if err := decompressSQLiteControlSnapshot(compressedPath, tempPath, manifest); err != nil {
		return err
	}
	if err := controlstore.ValidateSnapshot(ctx, tempPath); err != nil {
		return fmt.Errorf("validate restored SQLite control database: %w", err)
	}
	return activateRestoredControlDatabase(tempPath, targetPath)
}

func copyRestoreFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open restore artifact %s: %w", filepath.Base(source), err)
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	syncErr := output.Sync()
	closeErr := output.Close()
	return errors.Join(copyErr, syncErr, closeErr)
}

func decompressSQLiteControlSnapshot(sourcePath, targetPath string, manifest sqliteControlBackupManifest) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	decoder, err := zstd.NewReader(source, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
	if err != nil {
		return fmt.Errorf("open SQLite control snapshot decoder: %w", err)
	}
	defer decoder.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(target, hash), decoder)
	syncErr := target.Sync()
	closeErr := target.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return fmt.Errorf("decompress SQLite control snapshot: %w", err)
	}
	if written != manifest.Bytes {
		return fmt.Errorf("SQLite control snapshot size mismatch: got %d bytes, expected %d", written, manifest.Bytes)
	}
	if digest := hex.EncodeToString(hash.Sum(nil)); !strings.EqualFold(digest, manifest.SHA256) {
		return fmt.Errorf("SQLite control snapshot checksum mismatch")
	}
	return nil
}

func activateRestoredControlDatabase(tempPath, targetPath string) (retErr error) {
	backupPath := fmt.Sprintf("%s.pre-restore.%s", targetPath, time.Now().UTC().Format("2006-01-02T150405.000000000Z"))
	moved := make([]string, 0, 4)
	for _, suffix := range []string{"", "-wal", "-shm", ".wal"} {
		source := targetPath + suffix
		if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return fmt.Errorf("inspect existing control artifact: %w", err)
		}
		if err := os.Rename(source, backupPath+suffix); err != nil {
			for i := len(moved) - 1; i >= 0; i-- {
				_ = os.Rename(backupPath+moved[i], targetPath+moved[i])
			}
			return fmt.Errorf("preserve existing control artifact: %w", err)
		}
		moved = append(moved, suffix)
	}
	defer func() {
		if retErr == nil {
			return
		}
		_ = os.Remove(targetPath)
		for i := len(moved) - 1; i >= 0; i-- {
			_ = os.Rename(backupPath+moved[i], targetPath+moved[i])
		}
	}()
	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("publish restored SQLite control database: %w", err)
	}
	if err := os.Chmod(targetPath, 0o600); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(targetPath))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return fmt.Errorf("sync restored control directory: %w", err)
	}
	return nil
}

func downloadRestoreS3Object(ctx context.Context, sourceURI, destination string, s3Conf *worker.S3Config) error {
	bucket, key, err := parseRestoreS3URI(sourceURI)
	if err != nil {
		return err
	}
	region := ""
	if s3Conf != nil {
		region = s3Conf.Region
	}
	loadOptions := []func(*awsconfig.LoadOptions) error{awsconfig.WithRegion(region)}
	if s3Conf != nil && strings.TrimSpace(s3Conf.AccessKeyID) != "" {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s3Conf.AccessKeyID, s3Conf.SecretAccessKey, s3Conf.SessionToken)))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return err
	}
	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		if s3Conf == nil {
			return
		}
		if endpoint := strings.TrimSpace(s3Conf.Endpoint); endpoint != "" {
			if !strings.Contains(endpoint, "://") {
				scheme := "https"
				if !s3Conf.UseSSL {
					scheme = "http"
				}
				endpoint = scheme + "://" + endpoint
			}
			options.BaseEndpoint = aws.String(endpoint)
		}
		style := strings.ToLower(strings.TrimSpace(s3Conf.URLStyle))
		options.UsePathStyle = style == "path" || style == "path_style"
	})
	result, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return err
	}
	defer result.Body.Close()
	target, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(target, result.Body)
	syncErr := target.Sync()
	closeErr := target.Close()
	return errors.Join(copyErr, syncErr, closeErr)
}

func parseRestoreS3URI(value string) (string, string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "s3" || parsed.Host == "" {
		return "", "", fmt.Errorf("invalid S3 restore path %q", value)
	}
	key := strings.TrimPrefix(parsed.EscapedPath(), "/")
	key, err = url.PathUnescape(key)
	if err != nil || key == "" {
		return "", "", fmt.Errorf("invalid S3 restore object path %q", value)
	}
	return parsed.Host, key, nil
}

func isS3ObjectMissing(err error) bool {
	if err == nil {
		return false
	}
	var missing *types.NoSuchKey
	if errors.As(err, &missing) {
		return true
	}
	var apiError smithy.APIError
	return errors.As(err, &apiError) && (apiError.ErrorCode() == "NoSuchKey" || apiError.ErrorCode() == "NotFound")
}

// restoreDatabase imports a backup snapshot into a fresh DuckDB at targetPath.
// If targetPath already exists, it is renamed as a safety net.
func restoreDatabase(ctx context.Context, targetPath, sourcePath string, isS3 bool, s3Conf *worker.S3Config) error {
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
	store := database.NewStore(tempPath)
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
		fmt.Printf("  Existing DB renamed to %s\n", backupPath)
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

// findLatestLocalSnapshot finds the latest completed snapshot directory. New
// SQLite backup cycles commit with _COMPLETE; historical DuckDB exports are
// recognized by their load.sql/schema.sql pair.
func findLatestLocalSnapshot(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("could not read snapshot directory %s: %w", dir, err)
	}

	dirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() && completeLocalSnapshot(filepath.Join(dir, e.Name())) {
			dirs = append(dirs, e.Name())
		}
	}

	if len(dirs) == 0 {
		return "", fmt.Errorf("no completed snapshots found in %s", dir)
	}

	sort.Strings(dirs)
	return dirs[len(dirs)-1], nil
}

func completeLocalSnapshot(path string) bool {
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

func validateLocalTenantSnapshots(fromPath, snapshotName string, tenantIDs []string) error {
	var missing []string
	for _, tenantID := range tenantIDs {
		snapshotPath := filepath.Join(fromPath, "tenants", tenantID, snapshotName)
		if !completeLocalSnapshot(snapshotPath) {
			missing = append(missing, tenantID)
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return fmt.Errorf("snapshot %s is incomplete for %d active tenant(s): %s", snapshotName, len(missing), strings.Join(missing, ", "))
	}
	return nil
}

// discoverS3TenantBackupsFromControl derives S3 object prefixes from the
// restored control database because restore-backup intentionally does not list
// buckets. Split snapshots contain every active tenant, including the default;
// legacy snapshots contain only non-default tenant exports because the default
// tenant still lived in the shared database.
func discoverS3TenantBackupsFromControl(ctx context.Context, controlPath string) ([]string, error) {
	format, err := controlstore.InspectFormat(controlPath)
	if err != nil {
		return nil, err
	}
	if format == controlstore.FileSQLite {
		control, err := controlstore.Open(ctx, controlPath)
		if err != nil {
			return nil, fmt.Errorf("open restored SQLite control database: %w", err)
		}
		defer control.Close()
		tenantIDs, err := control.ListActiveTenantIDs(ctx)
		if err != nil {
			if isMissingTenantBackupSchema(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("list tenants in restored SQLite control database: %w", err)
		}
		ids := make([]string, 0, len(tenantIDs))
		for _, tenantID := range tenantIDs {
			ids = append(ids, tenantID.String())
		}
		return ids, nil
	}
	if format != controlstore.FileDuckDB {
		return nil, fmt.Errorf("restored control database has unsupported format %s", format)
	}
	control := database.NewStore(controlPath)
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
