//go:build linux

package pipeline

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// GetFileTimes extracts access time (atime), modification time (mtime), and change time (ctime) from a file.
// This function uses Linux-specific syscalls to access extended file time metadata.
//
// Returns:
//   - atime: Last access time
//   - mtime: Last modification time
//   - ctime: Last status change time (metadata change)
//   - err: Error if syscall fails
func GetFileTimes(info os.FileInfo) (atime, mtime, ctime time.Time, err error) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return time.Time{}, time.Time{}, time.Time{},
			fmt.Errorf("failed to get syscall.Stat_t from FileInfo")
	}

	// Linux uses Atim/Ctim fields (Timespec type)
	atime = time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
	ctime = time.Unix(stat.Ctim.Sec, stat.Ctim.Nsec)

	// Modification time is available via standard os.FileInfo interface
	mtime = info.ModTime()

	return atime, mtime, ctime, nil
}
