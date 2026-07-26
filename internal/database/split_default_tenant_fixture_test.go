package database

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

// TestLargeDefaultTenantSplitFixture is deliberately opt-in. It accepts a
// .db or low-memory .db.zst fixture and runs the migration in a helper
// subprocess so callers can measure peak RSS and restart behavior without
// placing client identifiers or raw rows in test output or artifacts.
func TestLargeDefaultTenantSplitFixture(t *testing.T) {
	if os.Getenv("HITKEEP_LARGE_DB_FIXTURE_HELPER") == "1" {
		runLargeDefaultTenantSplitFixtureHelper(t)
		return
	}
	fixture := strings.TrimSpace(os.Getenv("HITKEEP_LARGE_DB_FIXTURE"))
	if fixture == "" {
		t.Skip("set HITKEEP_LARGE_DB_FIXTURE to run the opt-in large database acceptance test")
	}
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("large database fixture is not readable: %v", err)
	}

	root := t.TempDir()
	workingDB := filepath.Join(root, "hitkeep.db")
	if err := materializeLargeFixture(fixture, workingDB); err != nil {
		t.Fatalf("materialize large database fixture: %v", err)
	}
	dataPath := filepath.Join(root, "data")
	metricsPath := filepath.Join(root, "metrics.json")

	run := func() int64 {
		cmd := exec.Command(os.Args[0], "-test.run=TestLargeDefaultTenantSplitFixture", "-test.v")
		env := append([]string{}, os.Environ()...)
		env = append(env,
			"HITKEEP_LARGE_DB_FIXTURE_HELPER=1",
			"HITKEEP_LARGE_DB_PATH="+workingDB,
			"HITKEEP_LARGE_DB_DATA_PATH="+dataPath,
			"HITKEEP_LARGE_DB_METRICS="+metricsPath,
			"HITKEEP_LARGE_DB_FIXTURE=",
		)
		cmd.Env = env
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		baseline, err := fixtureDirectorySize(root)
		if err != nil {
			t.Fatalf("measure fixture baseline disk usage: %v", err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatalf("start large database split helper: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		peak := baseline
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case err := <-done:
				if size, sizeErr := fixtureDirectorySize(root); sizeErr == nil && size > peak {
					peak = size
				}
				if err != nil {
					t.Fatalf("large database split helper failed: %v (%s)", err, sanitizeLargeFixtureFailure(output.String()))
				}
				return max(0, peak-baseline)
			case <-ticker.C:
				size, sizeErr := fixtureDirectorySize(root)
				if sizeErr != nil {
					continue
				}
				if size > peak {
					peak = size
				}
			}
		}
	}

	firstPeakAdditionalDisk := run()
	firstMetrics := readFixtureMetrics(t, metricsPath)
	if firstMetrics.ComparedTables != len(defaultTenantSplitFixtureTables) {
		t.Fatalf("large fixture compared %d tenant tables, want %d", firstMetrics.ComparedTables, len(defaultTenantSplitFixtureTables))
	}
	const maxAcceptedRSS = int64(1 << 30)
	if firstMetrics.PeakRSSBytes > maxAcceptedRSS {
		t.Fatalf(
			"large fixture split exceeded peak RSS budget: %d > %d bytes (control_migration=%d split=%d tenant_verify=%d)",
			firstMetrics.PeakRSSBytes,
			maxAcceptedRSS,
			firstMetrics.AfterControlMigrationRSS,
			firstMetrics.AfterSplitRSS,
			firstMetrics.AfterTenantVerifyRSS,
		)
	}
	maxAdditionalDisk := firstMetrics.InputBytes + defaultTenantSplitHeadroom
	if firstPeakAdditionalDisk > maxAdditionalDisk {
		t.Fatalf("large fixture split exceeded temporary disk budget: %d > %d bytes", firstPeakAdditionalDisk, maxAdditionalDisk)
	}
	firstSize := fixtureFileSize(t, workingDB)
	_ = run()
	secondMetrics := readFixtureMetrics(t, metricsPath)
	if secondMetrics.ComparedTables != 0 {
		t.Fatalf("idempotent restart repeated %d table comparisons", secondMetrics.ComparedTables)
	}
	if secondMetrics.InputBytes != firstSize {
		t.Fatalf("idempotent restart input size changed: %d != %d", secondMetrics.InputBytes, firstSize)
	}
	if secondMetrics.OutputBytes != firstSize || fixtureFileSize(t, workingDB) != firstSize {
		t.Fatalf("control database size changed on idempotent restart")
	}
	t.Logf("large fixture split: input_bytes=%d control_bytes=%d elapsed_ms=%d peak_rss_bytes=%d peak_additional_disk_bytes=%d", firstMetrics.InputBytes, firstMetrics.OutputBytes, firstMetrics.ElapsedMS, firstMetrics.PeakRSSBytes, firstPeakAdditionalDisk)
}

func fixtureDirectorySize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	return total, err
}

var (
	fixtureFailureLinePattern = regexp.MustCompile(`(?m)^\s+[^:\n]+\.go:\d+:\s+(.+)$`)
	fixtureUUIDPattern        = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	fixturePathPattern        = regexp.MustCompile(`/[^\s:]+`)
)

func sanitizeLargeFixtureFailure(output string) string {
	matches := fixtureFailureLinePattern.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return "helper output suppressed"
	}
	message := matches[len(matches)-1][1]
	message = fixtureUUIDPattern.ReplaceAllString(message, "<uuid>")
	message = fixturePathPattern.ReplaceAllString(message, "<path>")
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

type largeFixtureMetrics struct {
	InputBytes               int64 `json:"input_bytes"`
	OutputBytes              int64 `json:"output_bytes"`
	ElapsedMS                int64 `json:"elapsed_ms"`
	PeakRSSBytes             int64 `json:"peak_rss_bytes"`
	AfterControlMigrationRSS int64 `json:"after_control_migration_rss"`
	AfterSplitRSS            int64 `json:"after_split_rss"`
	AfterTenantVerifyRSS     int64 `json:"after_tenant_verify_rss"`
	ComparedTables           int   `json:"compared_tables"`
}

func runLargeDefaultTenantSplitFixtureHelper(t *testing.T) {
	ctx := context.Background()
	controlOptions := []StoreOption{WithMemoryLimit("768MiB"), WithThreads(1)}
	dataOptions := []StoreOption{WithMemoryLimit("512MiB"), WithThreads(1)}
	dbPath := os.Getenv("HITKEEP_LARGE_DB_PATH")
	dataPath := os.Getenv("HITKEEP_LARGE_DB_DATA_PATH")
	metricsPath := os.Getenv("HITKEEP_LARGE_DB_METRICS")
	start := time.Now()
	before, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat large fixture database: %v", err)
	}
	store, err := OpenDefaultSplitControlStore(ctx, dbPath, controlOptions...)
	if err != nil {
		t.Fatalf("migrate large fixture control schema: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close large fixture control database: %v", err)
	}
	afterControlMigrationRSS := fixturePeakRSS()
	logFixtureRSS(t, "after_control_migration")
	comparedTables := 0
	if err := runDefaultTenantSplitWithFaults(ctx, dbPath, dataPath, controlOptions, dataOptions, func(point string) error {
		if strings.HasPrefix(point, "after-target-fingerprint:") {
			comparedTables++
		}
		return nil
	}); err != nil {
		t.Fatalf("split large fixture database: %v", err)
	}
	control := NewStore(dbPath, controlOptions...)
	if err := control.Connect(); err != nil {
		t.Fatalf("reopen large fixture control database: %v", err)
	}
	complete, err := control.DefaultTenantSplitComplete(ctx)
	if err != nil || !complete {
		_ = control.Close()
		t.Fatalf("large fixture split markers incomplete: %v", err)
	}
	defaultID, err := control.GetDefaultTenantID(ctx)
	if err != nil {
		_ = control.Close()
		t.Fatalf("resolve large fixture default tenant: %v", err)
	}
	if err := control.Close(); err != nil {
		t.Fatalf("close large fixture control database after verification: %v", err)
	}
	afterSplitRSS := fixturePeakRSS()
	logFixtureRSS(t, "after_split_and_control_verify")
	tenant := NewStore(filepath.Join(dataPath, "tenants", defaultID.String(), "hitkeep.db"), dataOptions...)
	if err := tenant.Connect(); err != nil {
		t.Fatalf("open large fixture tenant database: %v", err)
	}
	if err := tenant.MigrateTenant(ctx); err != nil {
		_ = tenant.Close()
		t.Fatalf("migrate large fixture tenant database: %v", err)
	}
	if err := tenant.Close(); err != nil {
		t.Fatalf("close large fixture tenant database: %v", err)
	}
	afterTenantVerifyRSS := fixturePeakRSS()
	logFixtureRSS(t, "after_tenant_verify")
	after, err := os.Stat(dbPath)
	if err != nil {
		t.Fatalf("stat split large fixture database: %v", err)
	}
	maxRSS := fixturePeakRSS()
	metrics := largeFixtureMetrics{
		InputBytes:               before.Size(),
		OutputBytes:              after.Size(),
		ElapsedMS:                time.Since(start).Milliseconds(),
		PeakRSSBytes:             maxRSS,
		AfterControlMigrationRSS: afterControlMigrationRSS,
		AfterSplitRSS:            afterSplitRSS,
		AfterTenantVerifyRSS:     afterTenantVerifyRSS,
		ComparedTables:           comparedTables,
	}
	raw, err := json.Marshal(metrics)
	if err != nil {
		t.Fatalf("encode fixture metrics: %v", err)
	}
	if err := os.WriteFile(metricsPath, raw, 0o600); err != nil {
		t.Fatalf("write fixture metrics: %v", err)
	}
}

func logFixtureRSS(t *testing.T, phase string) {
	t.Helper()
	maxRSS := fixturePeakRSS()
	if maxRSS == 0 {
		return
	}
	t.Logf("large fixture phase=%s peak_rss_bytes=%d", phase, maxRSS)
}

func fixturePeakRSS() int64 {
	usage := &syscall.Rusage{}
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, usage); err != nil {
		return 0
	}
	maxRSS := int64(usage.Maxrss)
	if maxRSS > 0 && runtime.GOOS == "linux" {
		maxRSS *= 1024
	}
	return maxRSS
}

func materializeLargeFixture(source, target string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer output.Close()
	if strings.HasSuffix(strings.ToLower(source), ".zst") {
		decoder, err := zstd.NewReader(input, zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true))
		if err != nil {
			return err
		}
		defer decoder.Close()
		_, err = io.Copy(output, decoder)
		return err
	}
	_, err = io.Copy(output, input)
	return err
}

func readFixtureMetrics(t *testing.T, path string) largeFixtureMetrics {
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture metrics: %v", err)
	}
	var metrics largeFixtureMetrics
	if err := json.Unmarshal(raw, &metrics); err != nil {
		t.Fatalf("decode fixture metrics: %v", err)
	}
	return metrics
}

func fixtureFileSize(t *testing.T, path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat fixture output: %v", err)
	}
	return info.Size()
}
