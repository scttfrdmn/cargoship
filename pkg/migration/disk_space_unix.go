//go:build unix

package migration

import (
	"fmt"
	"syscall"
)

// getAvailableDiskSpace returns the available disk space for a path on Unix systems
func getAvailableDiskSpace(path string) (int64, error) {
	var stat syscall.Statfs_t
	err := syscall.Statfs(path, &stat)
	if err != nil {
		return 0, fmt.Errorf("failed to stat filesystem: %w", err)
	}

	// Available blocks * block size
	available := int64(stat.Bavail) * int64(stat.Bsize)
	return available, nil
}
