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

func TestRecoverUsesRootConfig(t *testing.T) {
	missingConfig := filepath.Join(t.TempDir(), "missing.yaml")
	for _, args := range [][]string{
		{"disable-2fa", "-email", "recovery@example.com"},
		{"restore-backup", "-from", t.TempDir()},
		{"restore-database-bundle", "-from", t.TempDir()},
		{"rebuild-default-tenant"},
		{"import-archives"},
	} {
		t.Run(args[0], func(t *testing.T) {
			for _, prefix := range [][]string{{"--config", missingConfig}, {"--config=" + missingConfig}} {
				root := NewRootCommand(slog.New(slog.NewTextHandler(io.Discard, nil)))
				root.SetIn(strings.NewReader("no\n"))
				root.SetOut(io.Discard)
				root.SetErr(io.Discard)
				commandArgs := append(append(prefix, "recover"), args...)
				err := ExecuteRoot(t.Context(), root, commandArgs)
				if err == nil || !strings.Contains(err.Error(), "load recovery configuration:") || !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("ExecuteRoot(%q) = %v, want missing configuration error", commandArgs, err)
				}
			}
		})
	}
}

func TestRecoverConfigurationPrecedence(t *testing.T) {
	for _, source := range []string{"file", "environment", "flag"} {
		t.Run(source, func(t *testing.T) {
			dir := t.TempDir()
			configFile := filepath.Join(dir, "recovery.yaml")
			fileDB, fileData := filepath.Join(dir, "file.db"), filepath.Join(dir, "file-data")
			if err := os.WriteFile(configFile, []byte("db-path: '"+fileDB+"'\ndata-path: '"+fileData+"'\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("HITKEEP_DB_PATH", "")
			t.Setenv("HITKEEP_DATA_PATH", "")
			wantDB, wantData := fileDB, fileData
			if source != "file" {
				wantDB, wantData = filepath.Join(dir, "env.db"), filepath.Join(dir, "env-data")
				t.Setenv("HITKEEP_DB_PATH", wantDB)
				t.Setenv("HITKEEP_DATA_PATH", wantData)
			}
			args := []string{"--config", configFile, "recover", "rebuild-default-tenant"}
			if source == "flag" {
				wantDB, wantData = filepath.Join(dir, "flag.db"), filepath.Join(dir, "flag-data")
				args = append(args, "-db", wantDB, "-data-path", wantData)
			}
			var stdout bytes.Buffer
			root := NewRootCommand(slog.New(slog.NewTextHandler(io.Discard, nil)))
			root.SetIn(strings.NewReader("no\n"))
			root.SetOut(&stdout)
			root.SetErr(io.Discard)
			if err := ExecuteRoot(t.Context(), root, args); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"Control DB: " + wantDB + "\n", "Data path:  " + wantData + "\n", "Aborted."} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("stdout = %q, want %q", stdout.String(), want)
				}
			}
			if _, err := os.Stat(wantDB); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("aborted recovery touched database: %v", err)
			}
		})
	}
}

func TestRecoverIgnoresProcessConfigArguments(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"hitkeep", "--config", filepath.Join(t.TempDir(), "missing.yaml")}
	t.Cleanup(func() { os.Args = originalArgs })
	configFile := filepath.Join(t.TempDir(), "recovery.yaml")
	if err := os.WriteFile(configFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	root := NewRootCommand(slog.New(slog.NewTextHandler(io.Discard, nil)))
	root.SetIn(strings.NewReader("no\n"))
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	dbPath, dataPath := filepath.Join(t.TempDir(), "control.db"), t.TempDir()
	err := ExecuteRoot(t.Context(), root, []string{"--config", configFile, "recover", "rebuild-default-tenant", "-db", dbPath, "-data-path", dataPath})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{dbPath, dataPath, "Aborted."} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want %q", stdout.String(), want)
		}
	}
}

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
	}, strings.NewReader("yes\n"), &stdout, &stderr, logger, "")

	recoveryErr, ok := errors.AsType[*RecoveryError](err)
	if !ok || recoveryErr.Code != 1 {
		t.Fatalf("Recover() error = %v, want RecoveryError code 1", err)
	}
	if !strings.Contains(stderr.String(), "Error restoring shared database:") || !strings.Contains(stderr.String(), context.Canceled.Error()) {
		t.Fatalf("stderr = %q, want canceled restore failure", stderr.String())
	}
}
