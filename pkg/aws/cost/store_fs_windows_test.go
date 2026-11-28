//go:build windows

package cost

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTransientFileError_Windows(t *testing.T) {
	for _, errno := range []syscall.Errno{syscall.ERROR_ACCESS_DENIED, errSharingViolation, errLockViolation} {
		assert.True(t, isTransientFileError(fmt.Errorf("op: %w", &os.LinkError{Op: "rename", Err: errno})), "errno %d", errno)
	}
	assert.False(t, isTransientFileError(syscall.ERROR_FILE_NOT_FOUND))
}

// TestRenameOverOpenFile_IsTransient pins the Windows behaviour the store
// works around: a rename over a file that os.Open holds fails with an error
// isTransientFileError accepts.
func TestRenameOverOpenFile_IsTransient(t *testing.T) {
	dir := t.TempDir()
	dst, src := filepath.Join(dir, "dst"), filepath.Join(dir, "src")
	require.NoError(t, os.WriteFile(dst, []byte("old"), 0600))
	require.NoError(t, os.WriteFile(src, []byte("new"), 0600))
	f, err := os.Open(dst)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	err = os.Rename(src, dst)
	require.Error(t, err, "Windows refuses to replace a file another handle has open")
	assert.True(t, isTransientFileError(err), "got %v", err)
}
