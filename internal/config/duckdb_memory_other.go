//go:build !darwin && !linux

package config

// physicalMemoryBytes reports the machine's physical RAM, or 0 when unknown.
// On platforms without a probe, DuckDB's own default stays in effect.
func physicalMemoryBytes() int64 {
	return 0
}
