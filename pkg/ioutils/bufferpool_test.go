package ioutils

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// TestBufferPool_Basic tests basic buffer pool operations
func TestBufferPool_Basic(t *testing.T) {
	pool := NewBufferPool(1024)

	// Get a buffer
	buf := pool.Get()
	if buf == nil {
		t.Fatal("Got nil buffer from pool")
	}

	if len(*buf) != 1024 {
		t.Errorf("Expected buffer size 1024, got %d", len(*buf))
	}

	// Use the buffer
	copy(*buf, []byte("test data"))

	// Return to pool
	pool.Put(buf)

	// Get another buffer (should be the same one from pool)
	buf2 := pool.Get()
	if buf2 == nil {
		t.Fatal("Got nil buffer from pool on second get")
	}

	// Should be full capacity
	if len(*buf2) != 1024 {
		t.Errorf("Expected buffer size 1024 after return, got %d", len(*buf2))
	}
}

// TestBufferPool_Concurrent tests concurrent buffer pool usage
func TestBufferPool_Concurrent(t *testing.T) {
	pool := NewBufferPool(4096)

	// Launch multiple goroutines using the pool
	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func() {
			buf := pool.Get()
			copy(*buf, []byte("concurrent test"))
			pool.Put(buf)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 100; i++ {
		<-done
	}
}

// TestReaderPool_Basic tests reader pool basic operations
func TestReaderPool_Basic(t *testing.T) {
	pool := NewReaderPool(8192)

	data := strings.NewReader("Test data for reader pool")

	// Get a reader
	reader := pool.Get(data)
	if reader == nil {
		t.Fatal("Got nil reader from pool")
	}

	// Read some data
	buf := make([]byte, 10)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if n != 10 {
		t.Errorf("Expected to read 10 bytes, got %d", n)
	}

	// Return to pool
	pool.Put(reader)

	// Get another reader
	data2 := strings.NewReader("New data")
	reader2 := pool.Get(data2)
	if reader2 == nil {
		t.Fatal("Got nil reader from pool on second get")
	}

	// Should be reading from new source
	buf2 := make([]byte, 8)
	n, err = reader2.Read(buf2)
	if err != nil {
		t.Fatalf("Second read failed: %v", err)
	}

	if string(buf2[:n]) != "New data" {
		t.Errorf("Expected 'New data', got '%s'", string(buf2[:n]))
	}

	pool.Put(reader2)
}

// TestWriterPool_Basic tests writer pool basic operations
func TestWriterPool_Basic(t *testing.T) {
	pool := NewWriterPool(8192)

	buf := &bytes.Buffer{}

	// Get a writer
	writer := pool.Get(buf)
	if writer == nil {
		t.Fatal("Got nil writer from pool")
	}

	// Write some data
	data := "Test data for writer pool"
	n, err := writer.WriteString(data)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	if n != len(data) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(data), n)
	}

	// Must flush before returning to pool
	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	if buf.String() != data {
		t.Errorf("Expected '%s', got '%s'", data, buf.String())
	}

	// Return to pool
	pool.Put(writer)

	// Get another writer
	buf2 := &bytes.Buffer{}
	writer2 := pool.Get(buf2)
	if writer2 == nil {
		t.Fatal("Got nil writer from pool on second get")
	}

	// Write to new destination
	data2 := "New data"
	_, err = writer2.WriteString(data2)
	if err != nil {
		t.Fatalf("Second write failed: %v", err)
	}

	if err := writer2.Flush(); err != nil {
		t.Fatalf("Second flush failed: %v", err)
	}

	if buf2.String() != data2 {
		t.Errorf("Expected '%s', got '%s'", data2, buf2.String())
	}

	pool.Put(writer2)
}

// TestStagedBufferPool_Selection tests size-based pool selection
func TestStagedBufferPool_Selection(t *testing.T) {
	sizes := []int{1024, 4096, 16384, 65536}
	pool := NewStagedBufferPool(sizes)

	tests := []struct {
		requestSize  int
		expectedSize int
	}{
		{512, 1024},      // Smaller than smallest pool
		{1024, 1024},     // Exact match
		{2000, 4096},     // Between sizes
		{4096, 4096},     // Exact match
		{10000, 16384},   // Between sizes
		{100000, 65536},  // Larger than largest pool
	}

	for _, tt := range tests {
		buf, size := pool.Get(tt.requestSize)
		if buf == nil {
			t.Errorf("Got nil buffer for request size %d", tt.requestSize)
			continue
		}

		if size != tt.expectedSize {
			t.Errorf("For request size %d, expected pool size %d, got %d",
				tt.requestSize, tt.expectedSize, size)
		}

		if len(*buf) != tt.expectedSize {
			t.Errorf("For request size %d, expected buffer length %d, got %d",
				tt.requestSize, tt.expectedSize, len(*buf))
		}

		pool.Put(buf, size)
	}
}

// TestStagedBufferPool_Reuse tests buffer reuse in staged pool
func TestStagedBufferPool_Reuse(t *testing.T) {
	sizes := []int{1024, 4096}
	pool := NewStagedBufferPool(sizes)

	// Get and return a buffer
	buf1, size1 := pool.Get(2000)
	if size1 != 4096 {
		t.Fatalf("Expected size 4096, got %d", size1)
	}

	// Mark the buffer with test data
	copy(*buf1, []byte("test marker"))

	// Return to pool
	pool.Put(buf1, size1)

	// Get the same size again - should reuse the buffer
	buf2, size2 := pool.Get(2000)
	if size2 != 4096 {
		t.Fatalf("Expected size 4096 on reuse, got %d", size2)
	}

	// Buffer should be reset to full capacity
	if len(*buf2) != 4096 {
		t.Errorf("Expected buffer length 4096 after reuse, got %d", len(*buf2))
	}
}

// TestDefaultBufferPool tests the default buffer pool
func TestDefaultBufferPool(t *testing.T) {
	buf := DefaultBufferPool.Get()
	if buf == nil {
		t.Fatal("Got nil buffer from default pool")
	}

	if len(*buf) != 32*1024 {
		t.Errorf("Expected default pool size 32KB, got %d", len(*buf))
	}

	DefaultBufferPool.Put(buf)
}

// TestDefaultReaderPool tests the default reader pool
func TestDefaultReaderPool(t *testing.T) {
	data := strings.NewReader("Test with default reader pool")

	reader := DefaultReaderPool.Get(data)
	if reader == nil {
		t.Fatal("Got nil reader from default pool")
	}

	buf := make([]byte, 28)
	_, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("Read from default reader pool failed: %v", err)
	}

	DefaultReaderPool.Put(reader)
}

// TestDefaultWriterPool tests the default writer pool
func TestDefaultWriterPool(t *testing.T) {
	buf := &bytes.Buffer{}

	writer := DefaultWriterPool.Get(buf)
	if writer == nil {
		t.Fatal("Got nil writer from default pool")
	}

	_, err := writer.WriteString("Test with default writer pool")
	if err != nil {
		t.Fatalf("Write to default writer pool failed: %v", err)
	}

	if err := writer.Flush(); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	DefaultWriterPool.Put(writer)
}

// TestDefaultStagedPool tests the default staged pool
func TestDefaultStagedPool(t *testing.T) {
	tests := []struct {
		requestSize  int
		expectedSize int
	}{
		{1024, 4 * 1024},                   // 4KB
		{16 * 1024, 32 * 1024},             // 32KB
		{128 * 1024, 256 * 1024},           // 256KB
		{512 * 1024, 1024 * 1024},          // 1MB
		{2 * 1024 * 1024, 4 * 1024 * 1024}, // 4MB
	}

	for _, tt := range tests {
		buf, size := DefaultStagedPool.Get(tt.requestSize)
		if buf == nil {
			t.Errorf("Got nil buffer for request size %d", tt.requestSize)
			continue
		}

		if size != tt.expectedSize {
			t.Errorf("For request size %d, expected pool size %d, got %d",
				tt.requestSize, tt.expectedSize, size)
		}

		DefaultStagedPool.Put(buf, size)
	}
}

// BenchmarkBufferPool_Allocation compares pool vs direct allocation
func BenchmarkBufferPool_Allocation(b *testing.B) {
	pool := NewBufferPool(4096)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := pool.Get()
		pool.Put(buf)
	}
}

// BenchmarkDirect_Allocation benchmarks direct allocation for comparison
func BenchmarkDirect_Allocation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = make([]byte, 4096)
	}
}

// BenchmarkReaderPool_Usage benchmarks reader pool usage
func BenchmarkReaderPool_Usage(b *testing.B) {
	pool := NewReaderPool(8192)
	data := strings.NewReader(strings.Repeat("test data ", 1000))
	buf := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = data.Seek(0, 0)
		reader := pool.Get(data)
		_, _ = reader.Read(buf)
		pool.Put(reader)
	}
}

// BenchmarkReaderDirect_Usage benchmarks direct reader creation
func BenchmarkReaderDirect_Usage(b *testing.B) {
	data := strings.NewReader(strings.Repeat("test data ", 1000))
	buf := make([]byte, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = data.Seek(0, 0)
		reader := bufio.NewReaderSize(data, 8192)
		_, _ = reader.Read(buf)
	}
}

// BenchmarkStagedPool_SmallRequests benchmarks staged pool with small buffers
func BenchmarkStagedPool_SmallRequests(b *testing.B) {
	sizes := []int{1024, 4096, 16384, 65536}
	pool := NewStagedBufferPool(sizes)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, size := pool.Get(2000)
		pool.Put(buf, size)
	}
}

// BenchmarkStagedPool_LargeRequests benchmarks staged pool with large buffers
func BenchmarkStagedPool_LargeRequests(b *testing.B) {
	sizes := []int{1024, 4096, 16384, 65536}
	pool := NewStagedBufferPool(sizes)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf, size := pool.Get(50000)
		pool.Put(buf, size)
	}
}
