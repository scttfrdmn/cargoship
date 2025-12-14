package pipeline

import (
	"errors"
	"io"
	"sync"
)

// BufferedPipe provides a pipe-like interface with a large internal buffer.
// Unlike io.Pipe's 4KB buffer, BufferedPipe allows the writer to work ahead
// by up to bufferSize bytes before blocking, enabling true concurrency between
// producer and consumer.
//
// Phase 2: This replaces io.Pipe() to eliminate the 751% serialization overhead
// (353% archiver + 398% uploader) caused by the tiny 4KB buffer.
type BufferedPipe struct {
	buffer    chan []byte // Channel of data chunks
	size      int         // Buffer size in bytes
	chunkSize int         // Size of each chunk

	writerErr error // Error from writer goroutine

	mu   sync.Mutex    // Protects error state and active writers
	once sync.Once     // Ensures close happens once
	done chan struct{} // Signals pipe is closed

	// Track active writers to prevent closing channel while writes are in progress
	activeWriters sync.WaitGroup
	closing       bool // Set to true when Close() is called

	// Pool for chunk reuse (Phase 1: Zero-Copy I/O optimization)
	chunkPool *sync.Pool
}

// BufferedPipeReader is the reading side of a BufferedPipe
type BufferedPipeReader struct {
	pipe       *BufferedPipe
	current    []byte // Current chunk being read
	currentPos int    // Position in current chunk
	closed     bool
}

// BufferedPipeWriter is the writing side of a BufferedPipe
type BufferedPipeWriter struct {
	pipe   *BufferedPipe
	closed bool
}

// NewBufferedPipe creates a new buffered pipe with the specified buffer size.
// The buffer is divided into chunks of chunkSize bytes to avoid allocating
// one giant buffer up front.
//
// Typical usage for Phase 2:
//
//	bufferSize: 64MB (allows archiver to work 64MB ahead)
//	chunkSize: 32KB (efficient for I/O, 2048 chunks total)
func NewBufferedPipe(bufferSize, chunkSize int) (*BufferedPipeReader, *BufferedPipeWriter) {
	numChunks := bufferSize / chunkSize
	if numChunks == 0 {
		numChunks = 1
	}

	pipe := &BufferedPipe{
		buffer:    make(chan []byte, numChunks),
		size:      bufferSize,
		chunkSize: chunkSize,
		done:      make(chan struct{}),
		chunkPool: &sync.Pool{
			New: func() interface{} {
				chunk := make([]byte, chunkSize)
				return &chunk
			},
		},
	}

	return &BufferedPipeReader{pipe: pipe}, &BufferedPipeWriter{pipe: pipe}
}

// Read implements io.Reader for BufferedPipeReader
func (r *BufferedPipeReader) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}

	for {
		// If we have data in current chunk, copy from it
		if r.current != nil && r.currentPos < len(r.current) {
			copied := copy(p[n:], r.current[r.currentPos:])
			r.currentPos += copied
			n += copied

			// If chunk exhausted, return it to pool and clear it
			if r.currentPos >= len(r.current) {
				// Restore full capacity before returning to pool
				fullChunk := r.current[:cap(r.current)]
				r.pipe.chunkPool.Put(&fullChunk)
				r.current = nil
				r.currentPos = 0
			}

			// If we filled the read buffer, return
			if n >= len(p) {
				return n, nil
			}

			// Continue to try to copy more from current chunk or get next chunk
			continue
		}

		// Need more data - read next chunk from channel
		chunk, ok := <-r.pipe.buffer
		if !ok {
			// Channel closed - check for writer error
			r.pipe.mu.Lock()
			err := r.pipe.writerErr
			r.pipe.mu.Unlock()

			if err != nil {
				return n, err
			}

			// Normal EOF
			if n > 0 {
				return n, nil
			}
			return 0, io.EOF
		}

		// Got new chunk - set it and continue loop to copy from it
		r.current = chunk
		r.currentPos = 0
	}
}

// Close implements io.Closer for BufferedPipeReader
func (r *BufferedPipeReader) Close() error {
	if r.closed {
		return nil
	}

	r.closed = true
	r.pipe.once.Do(func() {
		close(r.pipe.done)
	})

	// Return current chunk to pool if not yet returned
	if r.current != nil {
		// Restore full capacity before returning to pool
		fullChunk := r.current[:cap(r.current)]
		r.pipe.chunkPool.Put(&fullChunk)
		r.current = nil
	}

	// Drain remaining chunks to unblock writer and return them to pool
	for chunk := range r.pipe.buffer {
		// Restore full capacity before returning to pool
		fullChunk := chunk[:cap(chunk)]
		r.pipe.chunkPool.Put(&fullChunk)
	}

	return nil
}

// Write implements io.Writer for BufferedPipeWriter
func (w *BufferedPipeWriter) Write(p []byte) (n int, err error) {
	if w.closed {
		return 0, errors.New("write on closed pipe")
	}

	// Register this write operation
	w.pipe.mu.Lock()
	if w.pipe.closing {
		w.pipe.mu.Unlock()
		return 0, errors.New("write on closed pipe")
	}
	w.pipe.activeWriters.Add(1)
	w.pipe.mu.Unlock()

	// Ensure we deregister when done
	defer w.pipe.activeWriters.Done()

	// Check if pipe is closed
	select {
	case <-w.pipe.done:
		return 0, errors.New("write on closed pipe")
	default:
	}

	// Split write into chunks and send to buffer
	for n < len(p) {
		// Calculate chunk size (up to chunkSize bytes)
		end := n + w.pipe.chunkSize
		if end > len(p) {
			end = len(p)
		}

		// Get chunk from pool (zero-copy optimization)
		chunkPtr := w.pipe.chunkPool.Get().(*[]byte)
		fullChunk := *chunkPtr // Keep reference to full slice for pool return

		// Slice to the actual data size we need
		chunk := fullChunk[:end-n]
		copy(chunk, p[n:end])

		// Send chunk to buffer (blocks if buffer full - this is the backpressure)
		select {
		case w.pipe.buffer <- chunk:
			n = end
		case <-w.pipe.done:
			// Return full chunk to pool on error (not the sliced version)
			w.pipe.chunkPool.Put(&fullChunk)
			return n, errors.New("write on closed pipe")
		}
	}

	return n, nil
}

// Close implements io.Closer for BufferedPipeWriter
func (w *BufferedPipeWriter) Close() error {
	return w.CloseWithError(nil)
}

// CloseWithError closes the writer with an error.
// The error will be returned to the reader.
func (w *BufferedPipeWriter) CloseWithError(err error) error {
	if w.closed {
		return nil
	}

	w.closed = true

	// Store error for reader
	if err != nil {
		w.pipe.mu.Lock()
		w.pipe.writerErr = err
		w.pipe.mu.Unlock()
	}

	// Set closing flag and wait for active writers
	w.pipe.mu.Lock()
	w.pipe.closing = true
	w.pipe.mu.Unlock()

	// Wait for all active writes to complete before closing channel
	w.pipe.activeWriters.Wait()

	// Now safe to close buffer channel to signal EOF to reader
	close(w.pipe.buffer)

	// Signal done
	w.pipe.once.Do(func() {
		close(w.pipe.done)
	})

	return nil
}
