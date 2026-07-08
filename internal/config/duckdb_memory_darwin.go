//go:build darwin

package config

import "golang.org/x/sys/unix"

// physicalMemoryBytes reports the machine's physical RAM, or 0 when unknown.
func physicalMemoryBytes() int64 {
	size, err := unix.SysctlUint64("hw.memsize")
	if err != nil || size == 0 || size > uint64(1)<<62 {
		return 0
	}
	return int64(size)
}
