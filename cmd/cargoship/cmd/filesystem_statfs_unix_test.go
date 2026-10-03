//go:build integration && unix

package cmd

import (
	"syscall"
	"testing"
)

func integrationAvailableDiskBytes(t *testing.T, path string) uint64 {
	t.Helper()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		t.Fatalf("stat filesystem space: %v", err)
	}

	return stat.Bavail * uint64(stat.Bsize)
}
