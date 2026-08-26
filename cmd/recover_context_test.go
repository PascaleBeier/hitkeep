package hitkeepcmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoverRestoreBackupHonorsCanceledContext(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"hitkeep"}
	t.Cleanup(func() { os.Args = originalArgs })

	backup := t.TempDir()
	if err := os.MkdirAll(filepath.Join(backup, "shared", "snapshot"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdout, stderr bytes.Buffer
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	err := Recover(ctx, []string{
		"restore-backup",
		"-from", backup,
		"-db", filepath.Join(t.TempDir(), "hitkeep.db"),
		"-data-path", t.TempDir(),
	}, strings.NewReader("yes\n"), &stdout, &stderr, logger)

	recoveryErr, ok := errors.AsType[*RecoveryError](err)
	if !ok || recoveryErr.Code != 1 {
		t.Fatalf("Recover() error = %v, want RecoveryError code 1", err)
	}
	if !strings.Contains(stderr.String(), "Error restoring shared database:") || !strings.Contains(stderr.String(), context.Canceled.Error()) {
		t.Fatalf("stderr = %q, want canceled restore failure", stderr.String())
	}
}
