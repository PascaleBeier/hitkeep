package config

import (
	"regexp"
	"runtime"
	"testing"
)

const testGiB = int64(1) << 30

func TestDeriveDuckDBMemoryLimitBytes(t *testing.T) {
	tests := []struct {
		name        string
		goMemLimit  int64
		cgroupLimit int64
		physicalRAM int64
		want        int64
		wantOK      bool
	}{
		{
			name:        "GOMEMLIMIT wins and reserves process overhead",
			goMemLimit:  4 * testGiB,
			cgroupLimit: 0,
			physicalRAM: 64 * testGiB,
			want:        4*testGiB - 512*(1<<20),
			wantOK:      true,
		},
		{
			name:        "GOMEMLIMIT below floor clamps to 1GiB",
			goMemLimit:  1 * testGiB,
			physicalRAM: 64 * testGiB,
			want:        1 * testGiB,
			wantOK:      true,
		},
		{
			name:        "cgroup limit caps effective memory",
			cgroupLimit: 4 * testGiB,
			physicalRAM: 64 * testGiB,
			want:        2 * testGiB,
			wantOK:      true,
		},
		{
			name:        "physical RAM halved",
			physicalRAM: 8 * testGiB,
			want:        4 * testGiB,
			wantOK:      true,
		},
		{
			name:        "small hosts get the 1GiB floor",
			physicalRAM: 1 * testGiB,
			want:        1 * testGiB,
			wantOK:      true,
		},
		{
			name:        "huge hosts hit the 16GiB ceiling",
			physicalRAM: 128 * testGiB,
			want:        16 * testGiB,
			wantOK:      true,
		},
		{
			name:   "no detection leaves DuckDB default",
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := deriveDuckDBMemoryLimitBytes(tc.goMemLimit, tc.cgroupLimit, tc.physicalRAM)
			if ok != tc.wantOK {
				t.Fatalf("expected ok=%v, got %v", tc.wantOK, ok)
			}
			if ok && got != tc.want {
				t.Fatalf("expected %d bytes, got %d", tc.want, got)
			}
		})
	}
}

func TestLoadDerivesDuckDBDefaults(t *testing.T) {
	conf := load([]string{}, func(key, fallback string) string {
		return fallback
	})

	if conf.DuckDBThreads != runtime.GOMAXPROCS(0) {
		t.Fatalf("expected derived DuckDBThreads %d, got %d", runtime.GOMAXPROCS(0), conf.DuckDBThreads)
	}
	// Memory detection works on the platforms tests run on (linux, darwin).
	if !regexp.MustCompile(`^[0-9]+MiB$`).MatchString(conf.DuckDBMemoryLimit) {
		t.Fatalf("expected derived MiB memory limit, got %q", conf.DuckDBMemoryLimit)
	}
}

func TestLoadDuckDBExplicitValuesAndOptOut(t *testing.T) {
	env := map[string]string{
		"HITKEEP_DUCKDB_MEMORY_LIMIT": "3GB",
		"HITKEEP_DUCKDB_THREADS":      "6",
	}
	conf := load([]string{}, func(key, fallback string) string {
		if val, ok := env[key]; ok {
			return val
		}
		return fallback
	})
	if conf.DuckDBMemoryLimit != "3GB" || conf.DuckDBThreads != 6 {
		t.Fatalf("expected explicit values to pass through, got %q / %d", conf.DuckDBMemoryLimit, conf.DuckDBThreads)
	}

	conf = load([]string{}, func(key, fallback string) string {
		if key == "HITKEEP_DUCKDB_MEMORY_LIMIT" {
			return "none"
		}
		return fallback
	})
	if conf.DuckDBMemoryLimit != "" {
		t.Fatalf("expected 'none' to opt out of a memory limit, got %q", conf.DuckDBMemoryLimit)
	}
}
