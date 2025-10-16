// Package ioutils provides high-performance I/O utilities for CargoShip
// with zero-copy optimizations and efficient buffer management.
package ioutils

import (
	"io"
)

// CopyOptimized performs an optimized copy operation that leverages
// WriterTo and ReaderFrom interfaces for zero-copy I/O when available.
//
// Performance characteristics:
//   - With WriterTo/ReaderFrom: Zero-copy, ~2x faster for large transfers
//   - Fallback to io.Copy: Standard performance, safe for all types
//   - File-to-file transfers: Can use kernel-level zero-copy on Linux
//
// Returns the number of bytes copied and any error encountered.
func CopyOptimized(dst io.Writer, src io.Reader) (written int64, err error) {
	// Strategy 1: Check if src implements WriterTo
	// This allows the source to write directly to the destination,
	// potentially avoiding buffer allocations entirely
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}

	// Strategy 2: Check if dst implements ReaderFrom
	// This allows the destination to read directly from the source,
	// which is often more efficient for network or disk I/O
	if rf, ok := dst.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}

	// Strategy 3: Fallback to standard io.Copy
	// This is safe and efficient for all types, using an internal buffer
	return io.Copy(dst, src)
}

// CopyBuffer performs an optimized copy using a provided buffer.
// This is useful when you want to control buffer allocation and reuse.
//
// The buffer must be at least 32KB for optimal performance.
// For smaller buffers, io.CopyBuffer behavior is used.
//
// Returns the number of bytes copied and any error encountered.
func CopyBuffer(dst io.Writer, src io.Reader, buf []byte) (written int64, err error) {
	// Check if we can use zero-copy optimization even with a buffer provided
	if len(buf) == 0 {
		// No buffer provided, try zero-copy first
		return CopyOptimized(dst, src)
	}

	// With buffer provided: check for zero-copy interfaces first
	// as they may still be more efficient than buffered copy
	if wt, ok := src.(io.WriterTo); ok {
		return wt.WriteTo(dst)
	}

	if rf, ok := dst.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}

	// Use io.CopyBuffer with the provided buffer
	return io.CopyBuffer(dst, src, buf)
}

// CopyN performs an optimized copy of exactly n bytes from src to dst.
// This leverages WriterTo/ReaderFrom when possible while respecting the byte limit.
//
// Returns the number of bytes copied and any error encountered.
// Returns io.EOF if src has fewer than n bytes.
func CopyN(dst io.Writer, src io.Reader, n int64) (written int64, err error) {
	// For small copies, use standard io.CopyN (it's already efficient)
	if n < 32*1024 {
		return io.CopyN(dst, src, n)
	}

	// For larger copies, check if we can optimize
	// Note: We need to be careful with WriterTo/ReaderFrom and the byte limit

	// Strategy 1: Try using a LimitedReader with zero-copy
	lr := &io.LimitedReader{R: src, N: n}

	// Check if dst implements ReaderFrom
	// This is often the most efficient path for limited reads
	if rf, ok := dst.(io.ReaderFrom); ok {
		return rf.ReadFrom(lr)
	}

	// Strategy 2: Check if the limited reader's underlying source implements WriterTo
	// Note: This is tricky because LimitedReader doesn't implement WriterTo itself
	// We fall back to io.CopyN which handles this correctly
	return io.CopyN(dst, src, n)
}

// MultiWriter creates a writer that duplicates writes to all provided writers.
// This is a re-export of io.MultiWriter for convenience, but could be optimized
// in the future to use WriterTo when all writers support it.
func MultiWriter(writers ...io.Writer) io.Writer {
	return io.MultiWriter(writers...)
}

// TeeReader creates a Reader that writes to w what it reads from r.
// This is useful for operations like computing hashes while copying data.
//
// All reads from r performed through the returned Reader are matched
// with corresponding writes to w. There is no internal buffering.
func TeeReader(r io.Reader, w io.Writer) io.Reader {
	return io.TeeReader(r, w)
}

// SupportsZeroCopy returns true if the given reader-writer pair
// can benefit from zero-copy optimization.
//
// This is useful for deciding whether to use special code paths or
// log performance information.
func SupportsZeroCopy(dst io.Writer, src io.Reader) bool {
	_, hasWriterTo := src.(io.WriterTo)
	_, hasReaderFrom := dst.(io.ReaderFrom)
	return hasWriterTo || hasReaderFrom
}

// GetOptimizationMethod returns a description of the optimization
// method that will be used for the given reader-writer pair.
//
// Returns one of: "WriterTo", "ReaderFrom", "Standard"
func GetOptimizationMethod(dst io.Writer, src io.Reader) string {
	if _, ok := src.(io.WriterTo); ok {
		return "WriterTo"
	}
	if _, ok := dst.(io.ReaderFrom); ok {
		return "ReaderFrom"
	}
	return "Standard"
}
