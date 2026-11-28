//go:build unix

package pipeline

import (
	"os"
	"syscall"
	"testing"
)

// makeFIFO creates a FIFO at path, or skips the test where the platform has
// none. syscall.Mkfifo doesn't exist on Windows, hence the build tag.
func makeFIFO(t *testing.T, path string) {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	// A reader stuck opening the FIFO (the bug under test) is released by a
	// writer, so the test binary can still exit. With no reader this fails with
	// ENXIO at once instead of blocking.
	t.Cleanup(func() {
		if f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			_ = f.Close()
		}
	})
}
