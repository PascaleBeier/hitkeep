package hitkeepcmd

import (
	"bufio"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hitkeep/internal/config"
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
  -s3-access-key-id, -s3-secret-access-key, -s3-region, -s3-endpoint,
  -s3-url-style, -s3-use-ssl   (fall back to HITKEEP_S3_* env vars)

NOTE: HitKeep must be stopped before running recovery commands.
      DuckDB does not allow concurrent write access.`

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

	store := database.NewStore(*dbPath)
	if err := store.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not open database: %v\n", err)
		fmt.Fprintln(os.Stderr, "Make sure HitKeep is stopped before running recovery commands.")
		os.Exit(1)
	}
	defer store.Close()

	if err := store.Migrate(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: migration failed: %v\n", err)
		os.Exit(1)
	}

	// ---- Look up user ---------------------------------------------------
	user, err := store.GetUserByEmail(ctx, *email)
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
	hasTOTP, err := store.HasEnabledTOTP(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not check TOTP status: %v\n", err)
		os.Exit(1)
	}

	passkeys, err := store.ListUserPasskeys(ctx, user.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not list passkeys: %v\n", err)
		os.Exit(1)
	}
	recoveryCodesRemaining, err := store.CountActiveRecoveryCodes(ctx, user.ID)
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
	result, err := store.DisableUserMFA(ctx, user.ID)
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

func recoverRestoreBackup(args []string) {
	fs := flag.NewFlagSet("restore-backup", flag.ExitOnError)
	from := fs.String("from", "", "Backup source path (required) — local dir or s3://")
	snapshot := fs.String("snapshot", "", "Specific snapshot timestamp (default: latest)")
	dbPath := fs.String("db", "", "Target hitkeep.db path (defaults to server config value)")
	dataPath := fs.String("data-path", "", "Target data directory (defaults to server config value)")
	yes := fs.Bool("yes", false, "Skip confirmation prompt")

	s3AccessKeyID := fs.String("s3-access-key-id", "", "S3 access key ID")
	s3SecretAccessKey := fs.String("s3-secret-access-key", "", "S3 secret access key")
	s3Region := fs.String("s3-region", "", "S3 region")
	s3Endpoint := fs.String("s3-endpoint", "", "S3 custom endpoint")
	s3URLStyle := fs.String("s3-url-style", "", "S3 URL style: path or vhost")
	s3UseSSL := fs.Bool("s3-use-ssl", true, "S3 use SSL")

	if err := fs.Parse(args); err != nil {
		os.Exit(1)
	}

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

	// Resolve S3 flags from env vars if not set.
	if *s3AccessKeyID == "" {
		*s3AccessKeyID = conf.S3AccessKeyID
	}
	if *s3SecretAccessKey == "" {
		*s3SecretAccessKey = conf.S3SecretAccessKey
	}
	if *s3Region == "" {
		*s3Region = conf.S3Region
	}
	if *s3Endpoint == "" {
		*s3Endpoint = conf.S3Endpoint
	}
	if *s3URLStyle == "" {
		*s3URLStyle = conf.S3URLStyle
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
		fmt.Printf("Tenants:    %d non-default tenant(s)\n", len(tenantIDs))
		for _, id := range tenantIDs {
			fmt.Printf("  - %s\n", id)
		}
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
		s3Conf = &worker.S3Config{
			AccessKeyID:     *s3AccessKeyID,
			SecretAccessKey: *s3SecretAccessKey,
			Region:          *s3Region,
			Endpoint:        *s3Endpoint,
			URLStyle:        *s3URLStyle,
			UseSSL:          *s3UseSSL,
		}
	}

	// Restore shared DB.
	sharedSource := joinRestorePath(*from, "shared", snapshotName)
	if err := restoreDatabase(ctx, *dbPath, sharedSource, isS3Source, s3Conf); err != nil {
		fmt.Fprintf(os.Stderr, "Error restoring shared database: %v\n", err)
		exitCode = 1
	} else {
		fmt.Printf("Shared database restored to %s\n", *dbPath)
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

	backupPath, err := moveExistingDatabaseAside(targetPath)
	if err != nil {
		return err
	}
	if backupPath != "" {
		fmt.Printf("  Existing DB renamed to %s\n", backupPath)
	}

	// Import into a temporary database first so the final restored DB does not
	// depend on a WAL created by the recovery command itself.
	store := database.NewStore(tempPath)
	if err := store.Connect(); err != nil {
		return fmt.Errorf("could not create target database: %w", err)
	}

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

	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("activate restored database %s: %w", targetPath, err)
	}

	return nil
}

func s3ConfigForRestore(enabled bool, cfg *worker.S3Config) *database.S3SecretConfig {
	if !enabled {
		return nil
	}
	return cfg
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
	backup := fmt.Sprintf("%s.pre-restore.%s", targetPath, time.Now().UTC().Format("2006-01-02T150405Z"))
	renamed := false

	if _, err := os.Stat(targetPath); err == nil {
		if err := os.Rename(targetPath, backup); err != nil {
			return "", fmt.Errorf("could not rename existing database %s to %s: %w", targetPath, backup, err)
		}
		renamed = true
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat existing database %s: %w", targetPath, err)
	}

	walPath := targetPath + ".wal"
	if _, err := os.Stat(walPath); err == nil {
		walBackup := backup + ".wal"
		if renameErr := os.Rename(walPath, walBackup); renameErr != nil {
			return "", fmt.Errorf("could not rename existing WAL %s to %s: %w", walPath, walBackup, renameErr)
		}
		renamed = true
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("stat existing WAL %s: %w", walPath, err)
	}

	if renamed {
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
