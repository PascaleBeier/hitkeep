package database

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

type fakeDuckDBExtensionExecutor struct {
	execCalls []string
	results   map[string][]error
}

func (f *fakeDuckDBExtensionExecutor) ExecContext(_ context.Context, query string, _ ...any) (sql.Result, error) {
	f.execCalls = append(f.execCalls, query)
	if errs, ok := f.results[query]; ok && len(errs) > 0 {
		err := errs[0]
		f.results[query] = errs[1:]
		return nil, err
	}
	return nil, nil
}

func TestEnsureCoreExtensionLoadsInstalledExtension(t *testing.T) {
	t.Parallel()

	exec := &fakeDuckDBExtensionExecutor{}
	if err := EnsureCoreExtension(context.Background(), exec, "httpfs"); err != nil {
		t.Fatalf("ensure core extension: %v", err)
	}

	if len(exec.execCalls) != 1 || exec.execCalls[0] != "LOAD httpfs;" {
		t.Fatalf("unexpected exec calls: %#v", exec.execCalls)
	}
}

func TestLoadInstalledCoreExtensionReturnsLoadError(t *testing.T) {
	t.Parallel()

	exec := &fakeDuckDBExtensionExecutor{
		results: map[string][]error{
			"LOAD httpfs;": {errors.New("missing")},
		},
	}

	err := LoadInstalledCoreExtension(context.Background(), exec, "httpfs")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); !stringsContainsAll(got, []string{"load httpfs extension", "missing"}) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureCoreExtensionInstallsMissingExtension(t *testing.T) {
	t.Parallel()

	exec := &fakeDuckDBExtensionExecutor{
		results: map[string][]error{
			"LOAD excel;": {errors.New("missing")},
		},
	}

	if err := EnsureCoreExtension(context.Background(), exec, "excel"); err != nil {
		t.Fatalf("ensure core extension: %v", err)
	}

	want := []string{"LOAD excel;", "INSTALL excel;", "LOAD excel;"}
	if len(exec.execCalls) != len(want) {
		t.Fatalf("unexpected exec call count: got %d want %d (%#v)", len(exec.execCalls), len(want), exec.execCalls)
	}
	for i, query := range want {
		if exec.execCalls[i] != query {
			t.Fatalf("unexpected exec call %d: got %q want %q", i, exec.execCalls[i], query)
		}
	}
}

func TestEnsureCoreExtensionReturnsInstallError(t *testing.T) {
	t.Parallel()

	exec := &fakeDuckDBExtensionExecutor{
		results: map[string][]error{
			"LOAD excel;":    {errors.New("missing")},
			"INSTALL excel;": {errors.New("install failed")},
		},
	}

	err := EnsureCoreExtension(context.Background(), exec, "excel")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" || !stringsContainsAll(got, []string{"load excel extension", "install excel extension", "install failed"}) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureCoreExtensionRejectsEmptyName(t *testing.T) {
	t.Parallel()

	exec := &fakeDuckDBExtensionExecutor{}
	err := EnsureCoreExtension(context.Background(), exec, " ")
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "duckdb extension name is required"; got != want {
		t.Fatalf("unexpected error: got %q want %q", got, want)
	}
}

func stringsContainsAll(s string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

func TestConnectionsDisableImplicitExtensionFetching(t *testing.T) {
	ctx := context.Background()
	store := NewStore(":memory:")
	if err := store.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	// HitKeep loads everything it needs explicitly at bootstrap; implicit
	// extension fetching would mean silent network egress at query time.
	for _, setting := range []string{"autoinstall_known_extensions", "autoload_known_extensions", "allow_community_extensions"} {
		var value bool
		if err := store.DB().QueryRowContext(ctx, "SELECT current_setting(?)::BOOLEAN", setting).Scan(&value); err != nil {
			t.Fatalf("read %s: %v", setting, err)
		}
		if value {
			t.Errorf("expected %s to be disabled on new connections", setting)
		}
	}
}
