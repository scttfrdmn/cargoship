package ioutils

import (
	"bufio"
	"io"
	"sync"
)

// BufferPool provides efficient pooling of byte buffers to reduce allocations.
// It uses sync.Pool internally with size-aware allocation.
type BufferPool struct {
	pool     *sync.Pool
	baseSize int
}

// NewBufferPool creates a new buffer pool with the specified base size.
// Common sizes: 4KB, 8KB, 32KB, 64KB, 256KB, 1MB
func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		baseSize: size,
		pool: &sync.Pool{
			New: func() interface{} {
				buf := make([]byte, size)
				return &buf
			},
		},
	}
}

// Get retrieves a buffer from the pool.
// The buffer is guaranteed to be at least baseSize bytes.
func (bp *BufferPool) Get() *[]byte {
	return bp.pool.Get().(*[]byte)
}

// Put returns a buffer to the pool for reuse.
// The buffer should not be used after calling Put.
func (bp *BufferPool) Put(buf *[]byte) {
	// Reset the buffer slice to its full capacity
	*buf = (*buf)[:cap(*buf)]
	bp.pool.Put(buf)
}

// ReaderPool provides pooling for bufio.Reader instances
type ReaderPool struct {
	pool *sync.Pool
	size int
}

// NewReaderPool creates a new pool for bufio.Reader with specified buffer size
func NewReaderPool(size int) *ReaderPool {
	return &ReaderPool{
		size: size,
		pool: &sync.Pool{
			New: func() interface{} {
				return bufio.NewReaderSize(nil, size)
			},
		},
	}
}

// Get retrieves a bufio.Reader from the pool and resets it to read from r
func (rp *ReaderPool) Get(r io.Reader) *bufio.Reader {
	reader := rp.pool.Get().(*bufio.Reader)
	reader.Reset(r)
	return reader
}

// Put returns a bufio.Reader to the pool for reuse
func (rp *ReaderPool) Put(reader *bufio.Reader) {
	reader.Reset(nil) // Clear the underlying reader
	rp.pool.Put(reader)
}

// WriterPool provides pooling for bufio.Writer instances
type WriterPool struct {
	pool *sync.Pool
	size int
}

// NewWriterPool creates a new pool for bufio.Writer with specified buffer size
func NewWriterPool(size int) *WriterPool {
	return &WriterPool{
		size: size,
		pool: &sync.Pool{
			New: func() interface{} {
				return bufio.NewWriterSize(nil, size)
			},
		},
	}
}

// Get retrieves a bufio.Writer from the pool and resets it to write to w
func (wp *WriterPool) Get(w io.Writer) *bufio.Writer {
	writer := wp.pool.Get().(*bufio.Writer)
	writer.Reset(w)
	return writer
}

// Put returns a bufio.Writer to the pool for reuse.
// The writer must be flushed before calling Put.
func (wp *WriterPool) Put(writer *bufio.Writer) {
	writer.Reset(nil) // Clear the underlying writer
	wp.pool.Put(writer)
}

// StagedBufferPool provides multiple buffer pools of different sizes
// for optimal allocation based on data size.
type StagedBufferPool struct {
	pools map[int]*BufferPool
	sizes []int
}

// NewStagedBufferPool creates a buffer pool with multiple size tiers.
// Example sizes: [4KB, 32KB, 256KB, 1MB, 4MB]
func NewStagedBufferPool(sizes []int) *StagedBufferPool {
	pools := make(map[int]*BufferPool)
	for _, size := range sizes {
		pools[size] = NewBufferPool(size)
	}

	return &StagedBufferPool{
		pools: pools,
		sizes: sizes,
	}
}

// Get retrieves a buffer of at least the requested size.
// It selects the smallest pool that can satisfy the request.
func (sbp *StagedBufferPool) Get(size int) (*[]byte, int) {
	// Find the smallest buffer that can accommodate the requested size
	selectedSize := 0
	for _, poolSize := range sbp.sizes {
		if poolSize >= size {
			selectedSize = poolSize
			break
		}
	}

	// If no pool is large enough, use the largest available
	if selectedSize == 0 {
		selectedSize = sbp.sizes[len(sbp.sizes)-1]
	}

	return sbp.pools[selectedSize].Get(), selectedSize
}

// Put returns a buffer to the appropriate pool based on its size
func (sbp *StagedBufferPool) Put(buf *[]byte, size int) {
	if pool, exists := sbp.pools[size]; exists {
		pool.Put(buf)
	}
	// If size doesn't match any pool, let it be garbage collected
}

// DefaultBufferPool is a pre-configured buffer pool with 32KB buffers.
// This is suitable for most file I/O operations.
var DefaultBufferPool = NewBufferPool(32 * 1024)

// DefaultReaderPool is a pre-configured reader pool with 32KB buffers.
// This is suitable for most file reading operations.
var DefaultReaderPool = NewReaderPool(32 * 1024)

// DefaultWriterPool is a pre-configured writer pool with 32KB buffers.
// This is suitable for most file writing operations.
var DefaultWriterPool = NewWriterPool(32 * 1024)

// DefaultStagedPool is a pre-configured staged buffer pool with multiple sizes.
// Sizes: 4KB, 32KB, 256KB, 1MB, 4MB
var DefaultStagedPool = NewStagedBufferPool([]int{
	4 * 1024,        // 4KB - Small files, metadata
	32 * 1024,       // 32KB - Standard file operations
	256 * 1024,      // 256KB - Medium transfers
	1024 * 1024,     // 1MB - Large file chunks
	4 * 1024 * 1024, // 4MB - Very large transfers
})
