package main

import (
	"archive/tar"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyLegacyStorage(t *testing.T) {
	dataPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataPath, "hitkeep.db"), []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	metadataPath := writeLegacyStorageMetadata(t, dataPath)

	if err := verifyLegacyStorage(dataPath, metadataPath); err != nil {
		t.Fatalf("verifyLegacyStorage() error = %v", err)
	}

	if err := os.Mkdir(filepath.Join(dataPath, "tenants"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := verifyLegacyStorage(dataPath, metadataPath); err == nil || !strings.Contains(err.Error(), "split tenant data") {
		t.Fatalf("verifyLegacyStorage() error = %v, want split tenant data rejection", err)
	}
}

func TestRollbackSmokeUsesLegacyStorageVerifier(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "..", "scripts", "docker-smoke.sh"))
	if err != nil {
		t.Fatal(err)
	}
	rollback := string(script)
	start := strings.Index(rollback, `start_container "$previous_image" "$rollback_volume"`)
	if start < 0 {
		t.Fatal("rollback smoke does not start the restored volume with the historical image")
	}
	end := strings.Index(rollback[start:], `start_container "$image"`)
	if end < 0 {
		t.Fatal("rollback smoke does not resume the candidate volume")
	}
	rollback = rollback[start : start+end]
	if strings.Count(rollback, "verify_legacy_stopped_storage") != 1 {
		t.Fatal("restored legacy volume must use exactly one legacy stopped-storage verifier")
	}
	if strings.Contains(rollback, "verify_stopped_storage") {
		t.Fatal("restored legacy volume must not use the post-split stopped-storage verifier")
	}
}

func writeLegacyStorageMetadata(t *testing.T, dataPath string) string {
	t.Helper()
	path := filepath.Join(dataPath, "metadata.tar")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(file)
	if err := tw.WriteHeader(&tar.Header{Name: "data/hitkeep.db", Mode: 0o644, Uid: 65532, Gid: 65532, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
