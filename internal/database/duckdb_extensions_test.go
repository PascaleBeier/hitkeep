package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"os"
	"path/filepath"
	"testing"

	duckdb "github.com/duckdb/duckdb-go/v2"
)

func TestConnectionsDisableImplicitExtensionFetching(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// HitKeep loads everything it needs explicitly at bootstrap; implicit
	// extension fetching would mean silent network egress at query time.
	// preserve_insertion_order buffers full result and insert order in memory;
	// every user-visible ordering in HitKeep is an explicit ORDER BY.
	for _, setting := range []string{"autoinstall_known_extensions", "autoload_known_extensions", "allow_community_extensions", "preserve_insertion_order"} {
		var value bool
		if err := store.DB().QueryRowContext(ctx, "SELECT current_setting(?)::BOOLEAN", setting).Scan(&value); err != nil {
			t.Fatalf("read %s: %v", setting, err)
		}
		if value {
			t.Errorf("expected %s to be disabled on new connections", setting)
		}
	}
}

func TestS3SessionBootstrapsAWSSecretProvider(t *testing.T) {
	// DuckDB validates the credential chain while creating the secret. Keep the
	// test independent of developer or CI runner credential configuration.
	t.Setenv("AWS_ACCESS_KEY_ID", "test-access-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret-key")

	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	err := store.WithDuckDBSession(context.Background(), DuckDBSessionOptions{
		S3: &S3SecretConfig{Region: "eu-central-1"},
	}, func(_ *sql.Conn) error { return nil })
	if err != nil {
		t.Fatalf("prepare S3 session: %v", err)
	}
	rawDB, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })
	if err := WithDuckDBSession(t.Context(), rawDB, DuckDBSessionOptions{
		S3: &S3SecretConfig{Region: "eu-central-1"},
	}, func(_ *sql.Conn) error { return nil }); err != nil {
		t.Fatalf("prepare standalone backup session: %v", err)
	}
}

func TestBundledExtensionsLoadOffline(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(home, []byte("no home"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	path := filepath.Join(root, "offline.db")
	connector, err := duckdb.NewConnector(path, func(exec driver.ExecerContext) error {
		if _, err := exec.ExecContext(t.Context(), "SET custom_extension_repository='http://127.0.0.1:1'; SET extension_directory='"+escapeSQLString(filepath.Join(root, "empty-cache"))+"';", nil); err != nil {
			return err
		}
		return initializeCoreExtensions(t.Context(), exec)
	})
	if err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	t.Cleanup(func() { _ = db.Close() })
	for _, name := range duckDBCoreExtensions {
		var loaded bool
		if err := db.QueryRowContext(t.Context(), "SELECT loaded FROM duckdb_extensions() WHERE extension_name = ?", name).Scan(&loaded); err != nil {
			t.Fatal(err)
		}
		if !loaded {
			t.Errorf("%s not loaded", name)
		}
	}
	file := filepath.Join(root, "export.xlsx")
	if _, err := db.ExecContext(t.Context(), "COPY (SELECT 42 AS value) TO '"+escapeSQLString(file)+"' (FORMAT XLSX, HEADER true)"); err != nil {
		t.Fatal(err)
	}
	var value int
	if err := db.QueryRowContext(t.Context(), "SELECT value::INTEGER FROM read_xlsx('"+escapeSQLString(file)+"', header=true)").Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != 42 {
		t.Fatalf("XLSX round trip: got %d", value)
	}
}
