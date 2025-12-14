//go:build !disabled_for_now

/*
Package ioutils provides fallback NUMA implementations for all platforms.

NUMA support is currently disabled due to syscall portability issues.
This provides graceful fallback to standard buffer allocation.
*/
package ioutils

import "sync"

// NumaInfo provides information about NUMA configuration
type NumaInfo struct {
	Enabled   bool
	NodeCount int
	CPUCount  int
}

// NumaSupported always returns false on non-Linux platforms
func NumaSupported() bool {
	return false
}

// GetNumaInfo returns NUMA information (always disabled on non-Linux)
func GetNumaInfo() NumaInfo {
	return NumaInfo{
		Enabled:   false,
		NodeCount: 1,
		CPUCount:  0,
	}
}

// GetCurrentNumaNode always returns node 0 on non-Linux platforms
func GetCurrentNumaNode() (int, error) {
	return 0, nil
}

// NumaBuffer represents a buffer (non-NUMA on this platform)
type NumaBuffer struct {
	Data []byte
	Node int
}

// AllocateNumaBuffer allocates a standard buffer on non-Linux platforms
func AllocateNumaBuffer(size int) (*NumaBuffer, error) {
	return &NumaBuffer{
		Data: make([]byte, size),
		Node: 0,
	}, nil
}

// NumaBufferPool provides a standard buffer pool on non-Linux platforms
type NumaBufferPool struct {
	pool sync.Pool
	size int
}

// NewNumaBufferPool creates a standard buffer pool on non-Linux platforms
func NewNumaBufferPool(size int) *NumaBufferPool {
	p := &NumaBufferPool{
		size: size,
	}

	p.pool = sync.Pool{
		New: func() interface{} {
			buf := make([]byte, size)
			return &NumaBuffer{
				Data: buf,
				Node: 0,
			}
		},
	}

	return p
}

// Get retrieves a buffer from the pool
func (p *NumaBufferPool) Get() *NumaBuffer {
	return p.pool.Get().(*NumaBuffer)
}

// Put returns a buffer to the pool
func (p *NumaBufferPool) Put(buf *NumaBuffer) {
	if buf == nil {
		return
	}

	// Clear the buffer
	for i := range buf.Data {
		buf.Data[i] = 0
	}

	p.pool.Put(buf)
}

// GetBufferSize returns the size of buffers in the pool
func (p *NumaBufferPool) GetBufferSize() int {
	return p.size
}
