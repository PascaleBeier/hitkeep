package worker

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"hitkeep/internal/database"
)

// This integration check uses a disposable local S3 server with fixture
// credentials. It never accesses an operator's configured backup destination.
func TestBackupS3BundledExtensions(t *testing.T) {
	endpoint := os.Getenv("HITKEEP_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("set HITKEEP_TEST_S3_ENDPOINT to a disposable local MinIO server with a hitkeep-test bucket")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		t.Fatal("S3 integration endpoint must be HTTP on 127.0.0.1")
	}
	const accessKey = "hitkeep-test"
	const secretKey = "hitkeep-test-only"
	t.Setenv("AWS_ACCESS_KEY_ID", accessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", secretKey)
	t.Setenv("AWS_SESSION_TOKEN", "")
	home := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(home, []byte("no home"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	const bucket = "hitkeep-test"
	prefix := uuid.NewString()
	for _, mode := range []string{"static", "credential-chain"} {
		t.Run(mode, func(t *testing.T) {
			config := &S3Config{Region: "us-east-1", Endpoint: parsed.Host, URLStyle: "path", UseSSL: false}
			if mode == "static" {
				config.AccessKeyID, config.SecretAccessKey = accessKey, secretKey
			}
			store := database.NewStore(filepath.Join(t.TempDir(), "source.db"))
			if err := store.Connect(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = store.Close() })
			if _, err := store.DB().ExecContext(t.Context(), "CREATE TABLE probe AS SELECT 42 AS value"); err != nil {
				t.Fatal(err)
			}
			destination := "s3://" + bucket + "/" + prefix + "/" + mode
			worker := &BackupWorker{s3Config: config}
			if err := worker.exportDatabase(t.Context(), store, destination, true); err != nil {
				t.Fatal(err)
			}
			restored := database.NewStore(filepath.Join(t.TempDir(), "restored.db"))
			if err := restored.Connect(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = restored.Close() })
			if err := database.WithDuckDBSession(t.Context(), restored.DB(), database.DuckDBSessionOptions{
				S3: s3ConfigForSession(true, config),
			}, func(conn *sql.Conn) error {
				_, err := conn.ExecContext(t.Context(), "IMPORT DATABASE '"+destination+"'")
				return err
			}); err != nil {
				t.Fatal(err)
			}
			var value int
			if err := restored.DB().QueryRowContext(t.Context(), "SELECT value FROM probe").Scan(&value); err != nil {
				t.Fatal(err)
			}
			if value != 42 {
				t.Fatalf("restored value = %d, want 42", value)
			}
		})
	}
}
