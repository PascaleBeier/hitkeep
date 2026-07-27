package admin

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"

	"hitkeep/internal/api"
	"hitkeep/internal/database"
)

// handleGetSystemReport renders the operator-facing system report: a single
// markdown document with instance, configuration, storage, DuckDB memory,
// ingest, and feature information, suitable for pasting into a GitHub issue.
func (h *handler) handleGetSystemReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		cfg := h.ctx.Config

		var b strings.Builder
		writeLine := func(format string, args ...any) {
			fmt.Fprintf(&b, format+"\n", args...)
		}

		writeLine("# HitKeep System Report")
		writeLine("")
		writeLine("Generated: %s", time.Now().UTC().Format(time.RFC3339))
		writeLine("")
		writeLine("## Instance")
		writeLine("")
		writeLine("- Version: %s", cfg.Version)
		writeLine("- Runtime mode: %s", systemRuntimeMode(cfg))
		writeLine("- Uptime: %s", time.Since(h.ctx.StartedAt).Round(time.Second))
		writeLine("- Go version: %s", runtime.Version())
		writeLine("- OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH)
		writeLine("- CPUs: %d (GOMAXPROCS %d)", runtime.NumCPU(), runtime.GOMAXPROCS(0))
		if goMemLimit := debug.SetMemoryLimit(-1); goMemLimit > 0 && goMemLimit < int64(^uint64(0)>>1) {
			writeLine("- GOMEMLIMIT: %s", formatReportBytes(goMemLimit))
		}
		writeLine("")
		writeLine("## Configuration")
		writeLine("")
		writeLine("- DuckDB memory limit: %s", reportValueOrDefault(cfg.DuckDBMemoryLimit, "unlimited (DuckDB default: 80% of system RAM)"))
		if cfg.DuckDBThreads > 0 {
			writeLine("- DuckDB threads: %d", cfg.DuckDBThreads)
		} else {
			writeLine("- DuckDB threads: DuckDB default (all cores)")
		}
		writeLine("- Default data retention days: %d", cfg.DataRetentionDays)
		writeLine("")
		writeLine("## Storage")
		writeLine("")
		if fi, err := os.Stat(cfg.DBPath); err == nil {
			writeLine("- Shared database: %s", formatReportBytes(fi.Size()))
		} else {
			writeLine("- Shared database: size unavailable")
		}
		if tenants, err := h.ctx.Store.GetTenantList(ctx); err == nil {
			var tenantBytes int64
			for _, t := range tenants {
				if fi, err := os.Stat(filepath.Join(cfg.DataPath, "tenants", t.TenantID.String(), "hitkeep.db")); err == nil {
					tenantBytes += fi.Size()
				}
			}
			writeLine("- Tenant databases: %d (%s total)", len(tenants), formatReportBytes(tenantBytes))
		}
		if available, total, err := filesystemUsage(reportDiskPath(cfg.DataPath, cfg.DBPath)); err == nil {
			writeLine("- Disk: %s available of %s", formatReportBytes(available), formatReportBytes(total))
		}
		writeLine("")
		writeLine("## DuckDB Memory")
		writeLine("")
		var memoryStore *database.Store
		if h.ctx.TenantStores != nil {
			memoryStore, _ = h.ctx.TenantStores.ForTenant(ctx, uuid.Nil)
		}
		if stats, err := memoryStoreStats(ctx, memoryStore); err == nil {
			writeLine("| Tag | Memory | Temporary storage |")
			writeLine("| :-- | --: | --: |")
			for _, stat := range stats {
				if stat.MemoryBytes == 0 && stat.TempStorageBytes == 0 {
					continue
				}
				writeLine("| %s | %s | %s |", stat.Tag, formatReportBytes(stat.MemoryBytes), formatReportBytes(stat.TempStorageBytes))
			}
		} else {
			writeLine("Unavailable: %v", err)
		}
		writeLine("")
		writeLine("## Ingest (24h)")
		writeLine("")
		since := time.Now().UTC().Add(-24 * time.Hour)
		var counts database.RecentIngestCounts
		var err error
		if h.ctx.TenantStores == nil {
			err = fmt.Errorf("tenant analytics data plane is unavailable")
		} else {
			counts, err = h.ctx.TenantStores.GetRecentIngestCounts(ctx, since)
		}
		if err == nil {
			writeLine("- Hits: %d", counts.Hits)
			writeLine("- Events: %d", counts.Events)
		}
		if h.ctx.SystemCounters != nil {
			writeLine("- Rejections: %d", h.ctx.SystemCounters.Rejections.Load())
			writeLine("- Spam drops: %d", h.ctx.SystemCounters.Spam.Load())
		}
		writeLine("")
		writeLine("## Features")
		writeLine("")
		for _, feature := range systemFeatureStatuses(cfg, h.ctx.Mailer != nil) {
			state := "disabled"
			if feature.Enabled {
				state = "enabled"
			}
			if feature.Detail != "" {
				writeLine("- %s: %s (%s)", feature.Key, state, feature.Detail)
			} else {
				writeLine("- %s: %s", feature.Key, state)
			}
		}

		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(b.String()))
	}
}

func memoryStoreStats(ctx context.Context, store *database.Store) ([]api.DuckDBMemoryStat, error) {
	if store == nil {
		return nil, fmt.Errorf("tenant data plane unavailable")
	}
	return store.GetDuckDBMemoryStats(ctx)
}

func reportValueOrDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func reportDiskPath(dataPath, dbPath string) string {
	if strings.TrimSpace(dataPath) != "" {
		return dataPath
	}
	if strings.TrimSpace(dbPath) != "" {
		return filepath.Dir(dbPath)
	}
	return "."
}

func formatReportBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}
