package controlstore

import (
	"context"
	"path/filepath"
	"testing"
)

func TestBackupCreatesValidatedCompactSnapshotWithoutOverwriting(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "hitkeep.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.CreateUser(ctx, "backup@example.test", "hash"); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "control.db")
	info, err := store.Backup(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Bytes <= 0 || len(info.SHA256) != 64 {
		t.Fatalf("unexpected backup info: %+v", info)
	}
	if _, err := store.Backup(ctx, destination); err == nil {
		t.Fatal("backup overwrote an existing destination")
	}
	snapshot, err := Open(ctx, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	if count, err := snapshot.GetUserCount(ctx); err != nil || count != 1 {
		t.Fatalf("snapshot user count=%d err=%v", count, err)
	}
}
