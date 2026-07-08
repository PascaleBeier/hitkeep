package config

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
)

const (
	duckdbMemoryFloorBytes   = int64(1) << 30   // 1 GiB — below this, index writes under load are unsafe.
	duckdbMemoryCeilingBytes = int64(16) << 30  // 16 GiB — caching beyond this stops paying rent.
	duckdbGoOverheadBytes    = int64(512) << 20 // Reserved for the Go side when GOMEMLIMIT bounds the process.
	duckdbMiB                = int64(1) << 20
)

// resolveDuckDBDefaults derives safe DuckDB settings when the operator did
// not configure them, the way Go derives GOMAXPROCS: container-aware and
// bounded. An explicit HITKEEP_DUCKDB_MEMORY_LIMIT passes through; the
// literal "none" opts out and keeps DuckDB's own default (80% of system RAM).
func resolveDuckDBDefaults(conf *Config) {
	switch {
	case strings.EqualFold(strings.TrimSpace(conf.DuckDBMemoryLimit), "none"):
		conf.DuckDBMemoryLimit = ""
	case strings.TrimSpace(conf.DuckDBMemoryLimit) == "":
		if limitBytes, ok := deriveDuckDBMemoryLimitBytes(goMemLimitBytes(), cgroupMemoryLimitBytes(), physicalMemoryBytes()); ok {
			conf.DuckDBMemoryLimit = fmt.Sprintf("%dMiB", limitBytes/duckdbMiB)
		}
	}

	if conf.DuckDBThreads == 0 {
		conf.DuckDBThreads = runtime.GOMAXPROCS(0)
	}
}

// deriveDuckDBMemoryLimitBytes picks a per-database DuckDB memory limit from
// the strongest available signal: an operator-set GOMEMLIMIT, else the
// effective memory (cgroup limit if the process is containerized, physical
// RAM otherwise) halved and clamped to [1GiB, 16GiB].
func deriveDuckDBMemoryLimitBytes(goMemLimit, cgroupLimit, physicalRAM int64) (int64, bool) {
	if goMemLimit > 0 {
		return clampDuckDBMemory(goMemLimit - duckdbGoOverheadBytes), true
	}

	effective := physicalRAM
	if cgroupLimit > 0 && (effective == 0 || cgroupLimit < effective) {
		effective = cgroupLimit
	}
	if effective <= 0 {
		return 0, false
	}
	return clampDuckDBMemory(effective / 2), true
}

func clampDuckDBMemory(bytes int64) int64 {
	if bytes < duckdbMemoryFloorBytes {
		return duckdbMemoryFloorBytes
	}
	if bytes > duckdbMemoryCeilingBytes {
		return duckdbMemoryCeilingBytes
	}
	return bytes
}

// goMemLimitBytes reports the Go runtime's memory limit (the GOMEMLIMIT env
// var) or 0 when unset.
func goMemLimitBytes() int64 {
	limit := debug.SetMemoryLimit(-1)
	if limit <= 0 || limit == math.MaxInt64 {
		return 0
	}
	return limit
}

// cgroupMemoryLimitBytes reports the container memory limit on Linux, or 0
// when unlimited or not containerized.
func cgroupMemoryLimitBytes() int64 {
	if runtime.GOOS != "linux" {
		return 0
	}
	// cgroup v2, then v1.
	for _, path := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		value := strings.TrimSpace(string(raw))
		if value == "" || value == "max" {
			continue
		}
		limit, err := strconv.ParseInt(value, 10, 64)
		if err != nil || limit <= 0 {
			continue
		}
		// cgroup v1 reports a huge number when unlimited.
		if limit >= int64(1)<<60 {
			continue
		}
		return limit
	}
	return 0
}
