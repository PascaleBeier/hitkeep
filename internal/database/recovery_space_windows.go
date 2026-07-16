//go:build windows

package database

// Windows relies on the bundle write itself for the final free-space check.
// The recovery operation still remains fail-closed because source files are
// not mutated until the complete bundle and manifest have been fsynced.
func ensureRecoverySpace(_, _ string) error {
	return nil
}

func syncParentDirectory(string) error {
	return nil
}
