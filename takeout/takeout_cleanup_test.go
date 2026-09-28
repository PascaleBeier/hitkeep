package takeout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupExportFileRemovesOnlyScopedTakeouts(t *testing.T) {
	exportDir := filepath.Join(t.TempDir(), "exports")
	service := NewTakeoutService(nil, exportDir)

	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		t.Fatalf("mkdir export dir: %v", err)
	}

	validFile := filepath.Join(exportDir, "user_takeout_test.csv")
	if err := os.WriteFile(validFile, []byte("ok"), 0o600); err != nil {
		t.Fatalf("write valid takeout file: %v", err)
	}

	service.CleanupExportFile(validFile)
	if _, err := os.Stat(validFile); !os.IsNotExist(err) {
		t.Fatalf("expected valid takeout file to be removed, stat err=%v", err)
	}

	outsideFile := filepath.Join(t.TempDir(), "user_takeout_outside.csv")
	if err := os.WriteFile(outsideFile, []byte("ok"), 0o600); err != nil {
		t.Fatalf("write outside file: %v", err)
	}

	service.CleanupExportFile(outsideFile)
	if _, err := os.Stat(outsideFile); err != nil {
		t.Fatalf("expected outside file to remain, stat err=%v", err)
	}
}
