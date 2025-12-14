//go:build (linux || darwin || freebsd) && !js && !wasm

/*
Package ioutils provides memory-mapped file I/O optimizations for large file operations.

Memory mapping (mmap) allows files to be accessed as if they were in memory, providing
significant performance improvements for large file operations by leveraging the operating
system's virtual memory system and reducing system call overhead.
*/
package ioutils

import (
	"errors"
	"io"
	"os"
	"syscall"
)

const (
	// mmapThreshold is the minimum file size to benefit from memory mapping
	// Files smaller than this are better served by standard I/O
	mmapThreshold = 128 * 1024 * 1024 // 128MB
)

var (
	// ErrMmapNotSupported indicates memory mapping is not supported for the file
	ErrMmapNotSupported = errors.New("mmap: operation not supported for this file")

	// ErrMmapTooSmall indicates the file is too small to benefit from memory mapping
	ErrMmapTooSmall = errors.New("mmap: file too small for memory mapping")

	// ErrMmapInvalidFile indicates the file is not suitable for memory mapping
	ErrMmapInvalidFile = errors.New("mmap: invalid file for memory mapping")
)

// MmapReader provides memory-mapped read access to a file.
type MmapReader struct {
	data   []byte
	file   *os.File
	offset int64
	size   int64
}

// MmapSupported checks if a file is suitable for memory mapping.
// Files must be regular files above the threshold size.
func MmapSupported(file *os.File) bool {
	if file == nil {
		return false
	}

	// Check if it's a regular file
	info, err := file.Stat()
	if err != nil {
		return false
	}

	if !info.Mode().IsRegular() {
		return false
	}

	// Check if file is large enough to benefit from mmap
	if info.Size() < mmapThreshold {
		return false
	}

	return true
}

// NewMmapReader creates a memory-mapped reader for the given file.
// The file must be opened for reading and be a regular file above the threshold size.
func NewMmapReader(file *os.File) (*MmapReader, error) {
	if file == nil {
		return nil, ErrMmapInvalidFile
	}

	// Get file info
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	// Check if file is suitable for mmap
	if !info.Mode().IsRegular() {
		return nil, ErrMmapNotSupported
	}

	size := info.Size()
	if size < mmapThreshold {
		return nil, ErrMmapTooSmall
	}

	if size == 0 {
		return &MmapReader{
			data:   nil,
			file:   file,
			offset: 0,
			size:   0,
		}, nil
	}

	// Memory map the file for reading
	// Use PROT_READ for read-only access, MAP_SHARED for shared mapping
	data, err := syscall.Mmap(
		int(file.Fd()),
		0,                  // offset
		int(size),          // length
		syscall.PROT_READ,  // protection
		syscall.MAP_SHARED, // flags
	)
	if err != nil {
		return nil, err
	}

	return &MmapReader{
		data:   data,
		file:   file,
		offset: 0,
		size:   size,
	}, nil
}

// Read implements io.Reader for memory-mapped files.
func (m *MmapReader) Read(p []byte) (n int, err error) {
	if m.offset >= m.size {
		return 0, io.EOF
	}

	n = copy(p, m.data[m.offset:])
	m.offset += int64(n)

	if m.offset >= m.size {
		return n, io.EOF
	}

	return n, nil
}

// ReadAt implements io.ReaderAt for random access reads.
func (m *MmapReader) ReadAt(p []byte, off int64) (n int, err error) {
	if off < 0 {
		return 0, errors.New("mmap: negative offset")
	}

	if off >= m.size {
		return 0, io.EOF
	}

	n = copy(p, m.data[off:])
	if n < len(p) {
		return n, io.EOF
	}

	return n, nil
}

// Seek implements io.Seeker for the memory-mapped reader.
func (m *MmapReader) Seek(offset int64, whence int) (int64, error) {
	var newOffset int64

	switch whence {
	case io.SeekStart:
		newOffset = offset
	case io.SeekCurrent:
		newOffset = m.offset + offset
	case io.SeekEnd:
		newOffset = m.size + offset
	default:
		return 0, errors.New("mmap: invalid whence")
	}

	if newOffset < 0 {
		return 0, errors.New("mmap: negative position")
	}

	m.offset = newOffset
	return newOffset, nil
}

// Size returns the size of the memory-mapped file.
func (m *MmapReader) Size() int64 {
	return m.size
}

// Close unmaps the memory-mapped region and closes the underlying file.
func (m *MmapReader) Close() error {
	if m.data != nil {
		if err := syscall.Munmap(m.data); err != nil {
			return err
		}
		m.data = nil
	}

	// Note: We don't close the file here as it might be managed externally
	// The caller is responsible for closing the file if needed
	return nil
}

// CopyWithMmap attempts to copy a file using memory mapping for improved performance.
// Falls back to standard copy if memory mapping is not suitable.
func CopyWithMmap(dst io.Writer, src *os.File) (written int64, err error) {
	// Check if source file is suitable for mmap
	if !MmapSupported(src) {
		// Fall back to optimized copy
		return CopyOptimized(dst, src)
	}

	// Create memory-mapped reader
	mmapReader, err := NewMmapReader(src)
	if err != nil {
		// Fall back to optimized copy on mmap error
		return CopyOptimized(dst, src)
	}
	defer func() { _ = mmapReader.Close() }()

	// Use zero-copy transfer with memory-mapped source
	return CopyOptimized(dst, mmapReader)
}

// ReadFileWithMmap reads an entire file using memory mapping when beneficial.
// For small files, it falls back to standard file reading.
func ReadFileWithMmap(filename string) ([]byte, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	size := info.Size()

	// For small files, use standard reading
	if size < mmapThreshold {
		return os.ReadFile(filename)
	}

	// Use memory mapping for large files
	mmapReader, err := NewMmapReader(file)
	if err != nil {
		// Fall back to standard reading
		return os.ReadFile(filename)
	}
	defer func() { _ = mmapReader.Close() }()

	// Read entire file from memory-mapped region
	data := make([]byte, size)
	n, err := io.ReadFull(mmapReader, data)
	if err != nil && err != io.EOF {
		return nil, err
	}

	return data[:n], nil
}

// Note: Memory advise functions (Madvise) are platform-specific and implemented
// in separate files (mmap_linux.go, mmap_darwin.go, etc.) to handle platform differences.
// These functions provide hints to the kernel about memory access patterns for optimization.
