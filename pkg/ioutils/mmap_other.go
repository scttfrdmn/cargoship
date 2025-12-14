//go:build !(linux || darwin || freebsd) || js || wasm

/*
Package ioutils provides fallback memory-mapped file I/O for platforms without mmap support.

On platforms that don't support memory mapping (Windows, WASM, etc.), this file provides
stub implementations that always return "not supported" errors or fall back to standard I/O.
*/
package ioutils

import (
	"errors"
	"io"
	"os"
)

var (
	// ErrMmapNotSupported indicates memory mapping is not supported for the file
	ErrMmapNotSupported = errors.New("mmap: operation not supported for this file")

	// ErrMmapTooSmall indicates the file is too small to benefit from memory mapping
	ErrMmapTooSmall = errors.New("mmap: file too small for memory mapping")

	// ErrMmapInvalidFile indicates the file is not suitable for memory mapping
	ErrMmapInvalidFile = errors.New("mmap: invalid file for memory mapping")
)

// MmapReader provides a stub reader for platforms without mmap support.
type MmapReader struct {
	file   *os.File
	offset int64
	size   int64
}

// MmapSupported always returns false on platforms without mmap support.
func MmapSupported(file *os.File) bool {
	return false
}

// NewMmapReader returns an error on platforms without mmap support.
func NewMmapReader(file *os.File) (*MmapReader, error) {
	return nil, ErrMmapNotSupported
}

// Read implements io.Reader (stub implementation).
func (m *MmapReader) Read(p []byte) (n int, err error) {
	return 0, ErrMmapNotSupported
}

// ReadAt implements io.ReaderAt (stub implementation).
func (m *MmapReader) ReadAt(p []byte, off int64) (n int, err error) {
	return 0, ErrMmapNotSupported
}

// Seek implements io.Seeker (stub implementation).
func (m *MmapReader) Seek(offset int64, whence int) (int64, error) {
	return 0, ErrMmapNotSupported
}

// Size returns 0 on platforms without mmap support.
func (m *MmapReader) Size() int64 {
	return 0
}

// Close is a no-op on platforms without mmap support.
func (m *MmapReader) Close() error {
	return nil
}

// CopyWithMmap falls back to standard copy on platforms without mmap support.
func CopyWithMmap(dst io.Writer, src *os.File) (written int64, err error) {
	// Fall back to optimized copy
	return CopyOptimized(dst, src)
}

// ReadFileWithMmap falls back to standard file reading on platforms without mmap support.
func ReadFileWithMmap(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}
