package duckdbextensions

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/duckdb/duckdb-go/v2"
)

func TestBundleMatchesNativeDuckDB(t *testing.T) {
	db, err := sql.Open("duckdb", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var version string
	if err := db.QueryRowContext(t.Context(), "SELECT version()").Scan(&version); err != nil {
		t.Fatal(err)
	}
	lock, err := manifest()
	if err != nil {
		t.Fatal(err)
	}
	if version != lock.Version {
		t.Fatalf("native DuckDB is %s but bundled extensions are %s; refresh the bundle", version, lock.Version)
	}
	if _, err := db.ExecContext(t.Context(), "SET autoinstall_known_extensions=false; SET autoload_known_extensions=false; SET allow_community_extensions=false; SET custom_extension_repository='http://127.0.0.1:1';"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"httpfs", "aws", "excel"} {
		path, err := Path(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(t.Context(), "LOAD '"+strings.ReplaceAll(path, "'", "''")+"'"); err != nil {
			t.Fatalf("load %s: %v", name, err)
		}
	}
}

func TestBundleMatchesDuckDBDependency(t *testing.T) {
	command := exec.CommandContext(t.Context(), "go", "run", "./internal/duckdbextensions/update", "-check")
	command.Dir = filepath.Join("..", "..")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("stale or corrupt extension bundle: %v\n%s", err, output)
	}
}

func TestExtractionRepairsCorruptCache(t *testing.T) {
	directory := t.TempDir()
	path, err := extract(directory, "excel")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	// Simulate a new process inspecting a cache from an interrupted deployment.
	extraction.Lock()
	delete(extraction.ready, path)
	extraction.Unlock()
	if _, err := extract(directory, "excel"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() <= int64(len("corrupt")) {
		t.Fatal("corrupt extension was not replaced")
	}
	if _, err := extract(directory, "../httpfs"); err == nil {
		t.Fatal("accepted an extension outside the bundle")
	}
}

func TestLoadedExtensionsKeepOneProcessDirectory(t *testing.T) {
	path, err := Path("excel")
	if err != nil {
		t.Fatal(err)
	}
	if err := Configure(t.TempDir()); err == nil {
		t.Fatal("allowed a second native library location after loading")
	}
	again, err := Path("excel")
	if err != nil {
		t.Fatal(err)
	}
	if again != path {
		t.Fatalf("extension moved from %s to %s", path, again)
	}
}
