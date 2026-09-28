package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// S3SecretConfig defines the connection-local S3 credentials DuckDB should use.
//
//nolint:gosec // Runtime configuration struct — no hardcoded secrets.
type S3SecretConfig struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Region          string
	Endpoint        string
	URLStyle        string
	UseSSL          bool
}

type DuckDBSessionOptions struct {
	Excel bool
	S3    *S3SecretConfig
}

func (s *Store) WithDuckDBSession(ctx context.Context, opts DuckDBSessionOptions, fn func(*sql.Conn) error) error {
	return WithDuckDBSession(ctx, s.db, opts, fn)
}

// WithDuckDBSession runs fn on one physical connection after preparing the
// connection-local DuckDB state the caller needs.
func WithDuckDBSession(ctx context.Context, db *sql.DB, opts DuckDBSessionOptions, fn func(*sql.Conn) error) error {
	return WithPinnedConn(ctx, db, func(conn *sql.Conn) error {
		if err := prepareDuckDBSession(ctx, conn, opts); err != nil {
			return err
		}
		return fn(conn)
	})
}

func prepareDuckDBSession(ctx context.Context, conn *sql.Conn, opts DuckDBSessionOptions) error {
	if opts.Excel || opts.S3 != nil {
		if err := initializeSessionExtensions(ctx, conn); err != nil {
			return err
		}
	}
	if opts.S3 != nil {
		return ConfigureS3Secret(ctx, conn, opts.S3)
	}
	return nil
}

// ConfigureS3Secret creates a DuckDB S3 secret from the given config.
func ConfigureS3Secret(ctx context.Context, exec duckdbExtensionExecutor, cfg *S3SecretConfig) error {
	query := BuildS3SecretQuery(cfg)
	if query == "" {
		return nil
	}
	if _, err := exec.ExecContext(ctx, query); err != nil {
		return fmt.Errorf("configure S3 secret: %w", err)
	}
	return nil
}

// BuildS3SecretQuery generates a DuckDB CREATE SECRET statement for S3 authentication.
// Returns an empty string if cfg is nil.
func BuildS3SecretQuery(cfg *S3SecretConfig) string {
	if cfg == nil {
		return ""
	}

	var parts []string
	parts = append(parts, "TYPE s3")

	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		parts = append(parts, "PROVIDER config")
		parts = append(parts, fmt.Sprintf("KEY_ID '%s'", escapeSQLString(cfg.AccessKeyID)))
		parts = append(parts, fmt.Sprintf("SECRET '%s'", escapeSQLString(cfg.SecretAccessKey)))
		if cfg.SessionToken != "" {
			parts = append(parts, fmt.Sprintf("SESSION_TOKEN '%s'", escapeSQLString(cfg.SessionToken)))
		}
	} else {
		parts = append(parts, "PROVIDER credential_chain")
	}

	parts = append(parts, fmt.Sprintf("REGION '%s'", escapeSQLString(cfg.Region)))

	if cfg.Endpoint != "" {
		parts = append(parts, fmt.Sprintf("ENDPOINT '%s'", escapeSQLString(cfg.Endpoint)))
	}
	if cfg.URLStyle != "" {
		parts = append(parts, fmt.Sprintf("URL_STYLE '%s'", escapeSQLString(cfg.URLStyle)))
	}
	if !cfg.UseSSL {
		parts = append(parts, "USE_SSL false")
	}

	return fmt.Sprintf("CREATE OR REPLACE SECRET hitkeep_s3 (%s);", strings.Join(parts, ", "))
}

func escapeSQLString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}
