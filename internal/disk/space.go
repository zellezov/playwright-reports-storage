package disk

import (
	"fmt"

	"golang.org/x/sys/unix"
)

// HasSpace reports whether path has at least required free bytes.
func HasSpace(path string, required int64) (bool, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return false, fmt.Errorf("statfs %s: %w", path, err)
	}
	free := int64(stat.Bavail) * int64(stat.Bsize)
	return free >= required, nil
}

// Stats returns the used and free bytes for the filesystem containing path.
// Returns (0, 0) on error.
func Stats(path string) (used, free int64) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	total := int64(stat.Blocks) * int64(stat.Bsize)
	freeBytes := int64(stat.Bavail) * int64(stat.Bsize)
	return total - freeBytes, freeBytes
}
