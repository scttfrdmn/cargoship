package pipeline

import (
	"bytes"
	"io"
	"testing"
	"time"
)

func TestBufferedPipe_BasicReadWrite(t *testing.T) {
	pr, pw := NewBufferedPipe(1024*1024, 4096) // 1MB buffer, 4KB chunks

	// Write test data
	testData := []byte("Hello, World!")
	go func() {
		n, err := pw.Write(testData)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
		if n != len(testData) {
			t.Errorf("Write returned %d, want %d", n, len(testData))
		}
		_ = pw.Close()
	}()

	// Read and verify
	buf := make([]byte, 1024)
	n, err := pr.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read %d bytes, want %d", n, len(testData))
	}
	if !bytes.Equal(buf[:n], testData) {
		t.Errorf("Read data mismatch: got %q, want %q", buf[:n], testData)
	}

	_ = pr.Close()
}

func TestBufferedPipe_LargeData(t *testing.T) {
	// Test with data larger than buffer size
	pr, pw := NewBufferedPipe(64*1024, 8*1024) // 64KB buffer, 8KB chunks

	// Create 1MB of test data
	testData := make([]byte, 1024*1024)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// Write in goroutine
	go func() {
		n, err := pw.Write(testData)
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
		if n != len(testData) {
			t.Errorf("Write returned %d, want %d", n, len(testData))
		}
		_ = pw.Close()
	}()

	// Read all data
	var received bytes.Buffer
	buf := make([]byte, 8192)
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			received.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}

	// Verify
	if received.Len() != len(testData) {
		t.Errorf("Received %d bytes, want %d", received.Len(), len(testData))
	}
	if !bytes.Equal(received.Bytes(), testData) {
		t.Errorf("Data mismatch")
	}

	_ = pr.Close()
}

func TestBufferedPipe_CloseWithError(t *testing.T) {
	pr, pw := NewBufferedPipe(1024*1024, 4096)

	// Write some data then close with error
	testError := io.ErrUnexpectedEOF
	go func() {
		_, _ = pw.Write([]byte("test"))
		_ = pw.CloseWithError(testError)
	}()

	// Try to read all data
	buf := make([]byte, 1024)
	for {
		_, err := pr.Read(buf)
		if err != nil {
			// Should eventually get the error
			if err == testError {
				break // Expected error
			}
			if err != io.EOF {
				t.Errorf("Expected error %v, got %v", testError, err)
			}
			break
		}
	}

	_ = pr.Close()
}

func TestBufferedPipe_MultipleWrites(t *testing.T) {
	pr, pw := NewBufferedPipe(1024*1024, 4096)

	// Write multiple times
	chunks := []string{"chunk1", "chunk2", "chunk3", "chunk4"}
	go func() {
		for _, chunk := range chunks {
			_, err := pw.Write([]byte(chunk))
			if err != nil {
				t.Errorf("Write failed: %v", err)
			}
		}
		_ = pw.Close()
	}()

	// Read and verify
	var received bytes.Buffer
	buf := make([]byte, 16)
	for {
		n, err := pr.Read(buf)
		if n > 0 {
			received.Write(buf[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read failed: %v", err)
		}
	}

	expected := "chunk1chunk2chunk3chunk4"
	if received.String() != expected {
		t.Errorf("Received %q, want %q", received.String(), expected)
	}

	_ = pr.Close()
}

func TestBufferedPipe_ReaderCloseUnblocksWriter(t *testing.T) {
	pr, pw := NewBufferedPipe(1024, 512) // Small buffer to test blocking

	// Write large data that will exceed buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		data := make([]byte, 10*1024) // 10KB, much larger than 1KB buffer
		_, _ = pw.Write(data)
		_ = pw.Close()
	}()

	// Close reader immediately without reading
	_ = pr.Close()

	// Wait for writer to finish - it should unblock
	<-done
}

func BenchmarkBufferedPipe(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pr, pw := NewBufferedPipe(64*1024*1024, 32*1024) // 64MB, 32KB chunks

		go func() {
			_, _ = pw.Write(data)
			_ = pw.Close()
		}()

		buf := make([]byte, 32*1024)
		for {
			_, err := pr.Read(buf)
			if err == io.EOF {
				break
			}
		}
		_ = pr.Close()
	}
}

func TestBufferedPipe_ConcurrentWriteAndClose(t *testing.T) {
	// This test reproduces the race condition that caused panics in production:
	// Multiple goroutines writing while another goroutine closes the pipe.
	// Without proper synchronization, this causes "send on closed channel" panics.

	pr, pw := NewBufferedPipe(1024*1024, 4096) // 1MB buffer, 4KB chunks

	// Start multiple concurrent writers
	numWriters := 10
	writeSize := 100 * 1024 // 100KB per writer
	done := make(chan struct{})

	for i := 0; i < numWriters; i++ {
		go func(id int) {
			data := make([]byte, writeSize)
			for j := range data {
				data[j] = byte((id + j) % 256)
			}
			// This write might be in progress when Close() is called
			_, _ = pw.Write(data)
		}(i)
	}

	// Close the pipe while writes may still be in progress
	go func() {
		defer close(done)
		_ = pw.Close()
	}()

	// Read whatever data we can
	buf := make([]byte, 8192)
	for {
		_, err := pr.Read(buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Reader error is acceptable
			break
		}
	}

	_ = pr.Close()
	<-done // Wait for close to complete

	// If we reach here without panic, the fix is working
}

func BenchmarkBufferedPipe_vs_IOPipe(b *testing.B) {
	data := make([]byte, 1024*1024) // 1MB

	b.Run("BufferedPipe", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pr, pw := NewBufferedPipe(64*1024*1024, 32*1024)

			go func() {
				_, _ = pw.Write(data)
				_ = pw.Close()
			}()

			_, _ = io.Copy(io.Discard, pr)
			_ = pr.Close()
		}
	})

	b.Run("io.Pipe", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			pr, pw := io.Pipe()

			go func() {
				_, _ = pw.Write(data)
				_ = pw.Close()
			}()

			_, _ = io.Copy(io.Discard, pr)
			_ = pr.Close()
		}
	})
}

// TestBufferedPipePool tests the pool's basic Get/Put operations (Issue #34 Phase 1.1)
func TestBufferedPipePool(t *testing.T) {
	poolSize := 4
	bufSize := int64(1024 * 1024) // 1MB
	chunkSize := 32 * 1024        // 32KB

	pool := NewBufferedPipePool(poolSize, bufSize, chunkSize)

	// Get a pipe from the pool
	r, w := pool.Get()
	if r == nil || w == nil {
		t.Fatal("Expected non-nil reader and writer")
	}

	// Write some data
	testData := []byte("hello world")
	n, err := w.Write(testData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}

	// Close writer
	if err := w.Close(); err != nil {
		t.Fatalf("Close writer failed: %v", err)
	}

	// Read the data
	readBuf := make([]byte, len(testData))
	_, err = io.ReadFull(r, readBuf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if !bytes.Equal(readBuf, testData) {
		t.Errorf("Expected %q, got %q", testData, readBuf)
	}

	// Close reader
	if err := r.Close(); err != nil {
		t.Fatalf("Close reader failed: %v", err)
	}

	// Return to pool
	pool.Put(r, w)

	// Get it again and verify it's reusable
	r2, w2 := pool.Get()
	if r2 == nil || w2 == nil {
		t.Fatal("Expected non-nil reader and writer on second get")
	}

	// Write and read again
	_, err = w2.Write(testData)
	if err != nil {
		t.Fatalf("Second write failed: %v", err)
	}
	if err := w2.Close(); err != nil {
		t.Fatalf("Second close writer failed: %v", err)
	}

	readBuf2 := make([]byte, len(testData))
	_, err = io.ReadFull(r2, readBuf2)
	if err != nil {
		t.Fatalf("Second read failed: %v", err)
	}
	if !bytes.Equal(readBuf2, testData) {
		t.Errorf("Second read: expected %q, got %q", testData, readBuf2)
	}

	if err := r2.Close(); err != nil {
		t.Fatalf("Second close reader failed: %v", err)
	}

	pool.Put(r2, w2)
}

// TestBufferedPipePoolPutNil tests that Put handles nil inputs gracefully (Issue #34 Phase 1.1)
func TestBufferedPipePoolPutNil(t *testing.T) {
	pool := NewBufferedPipePool(2, 64*1024, 32*1024)

	// Should not panic
	pool.Put(nil, nil)

	r, w := pool.Get()
	pool.Put(r, nil)
	pool.Put(nil, w)
}

// TestBufferedPipePoolReset tests that pipes are properly reset on return (Issue #34 Phase 1.1)
func TestBufferedPipePoolReset(t *testing.T) {
	pool := NewBufferedPipePool(1, 64*1024, 32*1024)

	// First use
	r1, w1 := pool.Get()
	_, err := w1.Write([]byte("first"))
	if err != nil {
		t.Fatalf("First write failed: %v", err)
	}
	_ = w1.Close()
	_ = r1.Close()
	pool.Put(r1, w1)

	// Second use - should be clean
	r2, w2 := pool.Get()

	// Write new data
	testData := []byte("second")
	_, err = w2.Write(testData)
	if err != nil {
		t.Fatalf("Second write failed: %v", err)
	}
	_ = w2.Close()

	// Read should only get "second", not "first"
	readBuf := make([]byte, 10)
	n, err := r2.Read(readBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Second read failed: %v", err)
	}

	if n != len(testData) {
		t.Errorf("Expected to read %d bytes, got %d", len(testData), n)
	}

	if !bytes.Equal(readBuf[:n], testData) {
		t.Errorf("Expected %q, got %q", testData, readBuf[:n])
	}

	_ = r2.Close()
	pool.Put(r2, w2)
}

// TestBufferedPipePoolExhaustion tests pool behavior when exhausted (Issue #34 Phase 1.1)
func TestBufferedPipePoolExhaustion(t *testing.T) {
	poolSize := 2
	bufSize := int64(64 * 1024) // 64KB
	chunkSize := 32 * 1024      // 32KB

	pool := NewBufferedPipePool(poolSize, bufSize, chunkSize)

	// Get all pipes from pool
	pipes := make([]*BufferedPipeReader, poolSize)
	writers := make([]*BufferedPipeWriter, poolSize)
	for i := 0; i < poolSize; i++ {
		r, w := pool.Get()
		pipes[i] = r
		writers[i] = w
	}

	// Verify wait count is 0
	if pool.WaitCount() != 0 {
		t.Errorf("Expected wait count 0, got %d", pool.WaitCount())
	}

	// Try to get one more - should block until one is returned
	done := make(chan bool)
	go func() {
		r, w := pool.Get()
		// Return immediately
		pool.Put(r, w)
		done <- true
	}()

	// Wait a bit to ensure goroutine is blocked
	time.Sleep(10 * time.Millisecond)

	// Return one pipe to unblock
	pool.Put(pipes[0], writers[0])

	// Wait for goroutine to complete
	select {
	case <-done:
		// Success
	case <-time.After(1 * time.Second):
		t.Fatal("Goroutine did not unblock after returning pipe")
	}

	// Verify wait count increased
	if pool.WaitCount() == 0 {
		t.Error("Expected wait count > 0 after exhaustion")
	}

	// Clean up remaining pipes
	for i := 1; i < poolSize; i++ {
		pool.Put(pipes[i], writers[i])
	}
}
