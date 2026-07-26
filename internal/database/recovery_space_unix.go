//go:build !windows

package database

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func ensureRecoverySpace(root, databasePath string) error {
	required := int64(512 << 20)
	for _, path := range []string{databasePath, databasePath + ".wal"} {
		if info, err := os.Stat(path); err == nil {
			required += info.Size()
		}
	}
	if info, err := os.Stat(databasePath); err == nil {
		margin := info.Size() / 10
		if margin > 512<<20 {
			required += margin - (512 << 20)
		}
	}
	return ensureAvailableSpace(root, required)
}

func ensureAvailableSpace(root string, required int64) error {
	probe := root
	for {
		if _, err := os.Stat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return fmt.Errorf("resolve database recovery filesystem: %s", root)
		}
		probe = parent
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(probe, &stat); err != nil {
		return fmt.Errorf("inspect database recovery free space: %w", err)
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	availableBlocks := stat.Bavail
	// Statfs block sizes are non-negative; the platform types vary between
	// signed and unsigned integers, so normalize after the syscall succeeds.
	blockSize := uint64(stat.Bsize) //nolint:gosec // guarded platform syscall value
	available := maxInt64
	if blockSize == 0 || availableBlocks <= uint64(maxInt64)/blockSize {
		// The preceding bound proves the product fits in int64.
		available = int64(availableBlocks * blockSize) //nolint:gosec
	}
	if available < required {
		return fmt.Errorf("insufficient free space: need %d bytes, have %d", required, available)
	}
	return nil
}

func syncParentDirectory(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
