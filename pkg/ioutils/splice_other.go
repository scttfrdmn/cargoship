//go:build !linux
// +build !linux

/*
Package ioutils provides fallback implementations for non-Linux platforms.

On non-Linux platforms (Darwin/macOS, Windows, etc.), splice() is not available,
so we provide fallback implementations that use standard zero-copy optimizations.
*/
package ioutils

import (
	"errors"
	"io"
)

var (
	// ErrSpliceUnsupported indicates splice is not available on this platform
	ErrSpliceUnsupported = errors.New("splice: not supported on this platform")

	// ErrSpliceNotApplicable indicates the file descriptors don't support splice
	ErrSpliceNotApplicable = errors.New("splice: file descriptors not applicable for splice operation")
)

// SpliceSupported always returns false on non-Linux platforms.
func SpliceSupported(dst io.Writer, src io.Reader) bool {
	return false
}

// CopySplice always returns ErrSpliceUnsupported on non-Linux platforms.
func CopySplice(dst io.Writer, src io.Reader) (int64, error) {
	return 0, ErrSpliceUnsupported
}

// CopyOptimizedWithSplice falls back to CopyOptimized on non-Linux platforms.
// This ensures consistent API across all platforms with graceful degradation.
func CopyOptimizedWithSplice(dst io.Writer, src io.Reader) (int64, error) {
	// On non-Linux platforms, just use standard zero-copy optimization
	return CopyOptimized(dst, src)
}
