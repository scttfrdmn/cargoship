//go:build windows

package pipeline

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

// GetFileTimes extracts access time (atime), modification time (mtime), and creation time (ctime) from a file.
// This function uses Windows-specific syscalls to access extended file time metadata.
//
// Note: Windows tracks creation time (ctime) rather than status change time (as on Unix).
//
// Returns:
//   - atime: Last access time
//   - mtime: Last modification time (last write time)
//   - ctime: Creation time (Windows-specific, different from Unix ctime)
//   - err: Error if syscall fails
func GetFileTimes(info os.FileInfo) (atime, mtime, ctime time.Time, err error) {
	// Get Windows-specific file attributes
	stat, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return time.Time{}, time.Time{}, time.Time{},
			fmt.Errorf("failed to get Win32FileAttributeData from FileInfo")
	}

	// Convert Windows FILETIME to Go time.Time
	// FILETIME is 100-nanosecond intervals since January 1, 1601 UTC
	atime = time.Unix(0, stat.LastAccessTime.Nanoseconds())
	mtime = time.Unix(0, stat.LastWriteTime.Nanoseconds())
	ctime = time.Unix(0, stat.CreationTime.Nanoseconds())

	return atime, mtime, ctime, nil
}
