package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestStoreConnectInitializesConnectionSession(t *testing.T) {
	ctx := context.Background()
	store := NewStore(filepath.Join(t.TempDir(), "session.db"))
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := WithPinnedConn(ctx, store.DB(), func(conn *sql.Conn) error {
		var timezone string
		if err := conn.QueryRowContext(ctx, "SELECT current_setting('TimeZone')").Scan(&timezone); err != nil {
			return err
		}
		if timezone != "UTC" {
			t.Fatalf("expected timezone UTC, got %q", timezone)
		}

		var walCheckpoint string
		if err := conn.QueryRowContext(ctx, "SELECT current_setting('wal_autocheckpoint')").Scan(&walCheckpoint); err != nil {
			return err
		}
		if walCheckpoint == "" || walCheckpoint == "0 bytes" {
			t.Fatalf("expected wal_autocheckpoint to be configured, got %q", walCheckpoint)
		}

		return nil
	}); err != nil {
		t.Fatalf("check connection session: %v", err)
	}
}
