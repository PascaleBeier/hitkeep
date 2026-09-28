//go:build unix

package admin

import (
	"math"

	"golang.org/x/sys/unix"
)

func filesystemUsage(path string) (available int64, total int64, err error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}

	if stat.Bsize <= 0 {
		return 0, 0, nil
	}
	blockSize := uint64(stat.Bsize)

	return statfsBytes(stat.Bavail, blockSize), statfsBytes(stat.Blocks, blockSize), nil
}

func statfsBytes(blocks uint64, blockSize uint64) int64 {
	if blockSize == 0 {
		return 0
	}

	if blocks > uint64(math.MaxInt64)/blockSize {
		return math.MaxInt64
	}
	bytes := blocks * blockSize
	// #nosec G115 -- bytes is bounded above by math.MaxInt64 before conversion.
	return int64(bytes)
}
