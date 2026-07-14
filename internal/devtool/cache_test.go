package devtool

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCachePruneIsDryRunFirstAndManaged(t *testing.T) {
	workspace := t.TempDir()
	if output, err := exec.Command("git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateRoot)
	app, err := NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(stateRoot, "shared", "frontend", "old-snapshot")
	if err := os.MkdirAll(snapshot, 0o700); err != nil {
		t.Fatal(err)
	}
	content := filepath.Join(snapshot, ".complete")
	if err := os.WriteFile(content, []byte("old-snapshot\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(content, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(snapshot, old, old); err != nil {
		t.Fatal(err)
	}

	preview, err := app.PruneCache(24*time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if !preview.DryRun || len(preview.Candidates) != 1 || preview.Candidates[0].Path != snapshot {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if _, err := os.Stat(snapshot); err != nil {
		t.Fatalf("dry run removed snapshot: %v", err)
	}

	applied, err := app.PruneCache(24*time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	if applied.DryRun || len(applied.Removed) != 1 || applied.Removed[0].Path != snapshot {
		t.Fatalf("unexpected prune result: %+v", applied)
	}
	if _, err := os.Stat(snapshot); !os.IsNotExist(err) {
		t.Fatalf("snapshot was not removed: %v", err)
	}
	if _, err := os.Stat(workspace); err != nil {
		t.Fatalf("cache prune affected workspace: %v", err)
	}
}

func TestCachePruneRechecksFrontendSnapshotUseAfterLock(t *testing.T) {
	workspace := initTestRepository(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateRoot)
	app, err := NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(stateRoot, "shared", "frontend", "newly-linked")
	if err := os.MkdirAll(filepath.Join(snapshot, "node_modules"), 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(snapshot, ".complete")
	if err := os.WriteFile(marker, []byte("newly-linked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(marker, old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(snapshot, "node_modules"), old, old); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(snapshot, old, old); err != nil {
		t.Fatal(err)
	}
	report, err := app.CacheStatus()
	if err != nil {
		t.Fatal(err)
	}
	dashboard := filepath.Join(workspace, "frontend", "dashboard")
	if err := os.MkdirAll(dashboard, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(snapshot, "node_modules"), filepath.Join(dashboard, "node_modules")); err != nil {
		t.Fatal(err)
	}

	result, err := app.pruneCacheReport(report, 24*time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 {
		t.Fatalf("newly linked snapshot was removed: %+v", result.Removed)
	}
	if _, err := os.Stat(snapshot); err != nil {
		t.Fatalf("newly linked snapshot is missing: %v", err)
	}
}
