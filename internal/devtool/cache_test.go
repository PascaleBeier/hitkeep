package devtool

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCachePruneIsDryRunFirstAndManaged(t *testing.T) {
	workspace := t.TempDir()
	if output, err := exec.CommandContext(t.Context(), "git", "init", "--quiet", workspace).CombinedOutput(); err != nil {
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

func TestCacheStatusAndPruneDockerComposeCacheVolumes(t *testing.T) {
	workspace := initTestRepository(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateRoot)
	app, err := NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}

	bin := t.TempDir()
	removed := filepath.Join(t.TempDir(), "removed")
	state := filepath.Join(t.TempDir(), "docker-state")
	if err := os.WriteFile(state, []byte("dangling\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	docker := filepath.Join(bin, "docker")
	script := strings.ReplaceAll(`#!/bin/sh
case "$1:$2" in
volume:ls)
	case " $* " in
	*" dangling=true "*" label=com.docker.compose.project "*) ;;
	*) exit 1 ;;
	esac
	if [ "$(cat "$HK_DOCKER_CACHE_STATE")" = linked ]; then
		exit 0
	fi
	printf '%s\n' stale-go-build current-go-mod data-volume archive-volume foreign-go-mod unlabeled
	;;
volume:inspect)
	cat <<'JSON'
{"Name":"stale-go-build","CreatedAt":"2020-01-01T00:00:00Z","Labels":{"com.docker.compose.project":"hitkeep-deadbeef","com.docker.compose.volume":"go-build"}}
{"Name":"current-go-mod","CreatedAt":"2020-01-01T00:00:00Z","Labels":{"com.docker.compose.project":"CURRENT_PROJECT","com.docker.compose.volume":"go-mod"}}
{"Name":"data-volume","CreatedAt":"2020-01-01T00:00:00Z","Labels":{"com.docker.compose.project":"hitkeep-deadbeef","com.docker.compose.volume":"data"}}
{"Name":"archive-volume","CreatedAt":"2020-01-01T00:00:00Z","Labels":{"com.docker.compose.project":"hitkeep-deadbeef","com.docker.compose.volume":"archive"}}
{"Name":"foreign-go-mod","CreatedAt":"2020-01-01T00:00:00Z","Labels":{"com.docker.compose.project":"other-project","com.docker.compose.volume":"go-mod"}}
{"Name":"unlabeled","CreatedAt":"2020-01-01T00:00:00Z","Labels":{}}
JSON
	;;
volume:rm)
	printf '%s\n' "$3" >> "$HK_DOCKER_CACHE_REMOVED"
	printf '%s\n' "$3"
	;;
esac
`, "CURRENT_PROJECT", app.workspace.ComposeProject)
	if err := os.WriteFile(docker, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HK_DOCKER_CACHE_REMOVED", removed)
	t.Setenv("HK_DOCKER_CACHE_STATE", state)

	report, err := app.CacheStatus()
	if err != nil {
		t.Fatal(err)
	}
	var dockerEntries []CacheEntry
	for _, entry := range report.Entries {
		if entry.Kind == dockerComposeCacheKind {
			dockerEntries = append(dockerEntries, entry)
		}
	}
	if len(dockerEntries) != 1 || dockerEntries[0].Key != "stale-go-build" || !dockerEntries[0].Prunable || dockerEntries[0].InUse || dockerEntries[0].Bytes != 0 {
		t.Fatalf("unexpected Docker cache entries: %+v", dockerEntries)
	}

	preview, err := app.PruneCache(time.Hour, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Candidates) != 1 || preview.Candidates[0].Key != "stale-go-build" {
		t.Fatalf("unexpected Docker cache preview: %+v", preview)
	}
	if _, err := os.Stat(removed); !os.IsNotExist(err) {
		t.Fatalf("dry run removed Docker volume: %v", err)
	}

	applied, err := app.PruneCache(time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Removed) != 1 || applied.Removed[0].Key != "stale-go-build" {
		t.Fatalf("unexpected Docker cache prune: %+v", applied)
	}
	contents, err := os.ReadFile(removed)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "stale-go-build\n" {
		t.Fatalf("unexpected Docker volume removal: %q", contents)
	}

	recheckReport, err := app.CacheStatus()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(state, []byte("linked\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	skipped, err := app.pruneCacheReport(recheckReport, time.Hour, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped.Removed) != 0 {
		t.Fatalf("newly linked Docker volume was removed: %+v", skipped.Removed)
	}
	contents, err = os.ReadFile(removed)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "stale-go-build\n" {
		t.Fatalf("linked Docker volume was removed: %q", contents)
	}
}

func TestCacheStatusIgnoresUnavailableDocker(t *testing.T) {
	workspace := initTestRepository(t)
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateRoot)
	app, err := NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := app.CacheStatus(); err != nil {
		t.Fatalf("cache status should stay available without Docker: %v", err)
	}
}

func TestCacheStatusKeepsCurrentManagedToolchains(t *testing.T) {
	workspace := initTestRepository(t)
	writeTestToolchainConfig(t, workspace)
	stateRoot := filepath.Join(t.TempDir(), "state")
	t.Setenv("HK_STATE_DIR", stateRoot)
	app, err := NewApp(workspace)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := app.managedToolchainPaths()
	if err != nil {
		t.Fatal(err)
	}
	oldToolchain := filepath.Join(paths.Root, "toolchains", "go-0.0.1-"+paths.Platform)
	for _, path := range []string{paths.GoRoot, paths.NodeRoot, oldToolchain} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	report, err := app.CacheStatus()
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]CacheEntry{}
	for _, entry := range report.Entries {
		if entry.Kind == "managed-toolchain" {
			statuses[entry.Path] = entry
		}
	}
	for _, current := range []string{paths.GoRoot, paths.NodeRoot} {
		entry, ok := statuses[current]
		if !ok || !entry.InUse || entry.Prunable {
			t.Fatalf("current managed toolchain is not retained: %+v", entry)
		}
	}
	if entry := statuses[oldToolchain]; entry.InUse || !entry.Prunable {
		t.Fatalf("old managed toolchain is not prunable: %+v", entry)
	}
}
