//nolint:errcheck // Test file - errors in cleanup operations are acceptable
package ioutils

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// TestMmapSupported verifies memory mapping suitability detection
func TestMmapSupported(t *testing.T) {
	// Test with nil file
	if MmapSupported(nil) {
		t.Error("Expected MmapSupported to return false for nil file")
	}

	// Create a small temporary file (below threshold)
	smallFile, err := os.CreateTemp("", "mmap_test_small_*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(smallFile.Name())
	defer smallFile.Close()

	// Write less than 128MB
	testData := bytes.Repeat([]byte("test"), 1024*1024) // 4MB
	if _, err := smallFile.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Small file should not be supported
	if MmapSupported(smallFile) {
		t.Error("Expected MmapSupported to return false for small file")
	}

	// Create a large temporary file (above threshold)
	largeFile, err := os.CreateTemp("", "mmap_test_large_*")
	if err != nil {
		t.Fatalf("Failed to create large temp file: %v", err)
	}
	defer os.Remove(largeFile.Name())
	defer largeFile.Close()

	// Write more than 128MB
	chunk := bytes.Repeat([]byte("X"), 1024*1024) // 1MB chunk
	for i := 0; i < 129; i++ {
		if _, err := largeFile.Write(chunk); err != nil {
			t.Fatalf("Failed to write chunk %d: %v", i, err)
		}
	}

	// Large file should be supported
	if !MmapSupported(largeFile) {
		t.Error("Expected MmapSupported to return true for large file")
	}
}

// TestNewMmapReader verifies MmapReader creation and basic operations
func TestNewMmapReader(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Create a large temporary file
	file, err := os.CreateTemp("", "mmap_test_reader_*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()

	// Write test data above threshold
	testData := bytes.Repeat([]byte("test data for mmap reader\n"), 5*1024*1024) // ~130MB
	if _, err := file.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Reopen file for reading
	file.Close()
	file, err = os.Open(file.Name())
	if err != nil {
		t.Fatalf("Failed to reopen file: %v", err)
	}

	// Create MmapReader
	reader, err := NewMmapReader(file)
	if err != nil {
		t.Fatalf("Failed to create MmapReader: %v", err)
	}
	defer reader.Close()

	// Verify size
	if reader.Size() != int64(len(testData)) {
		t.Errorf("Expected size %d, got %d", len(testData), reader.Size())
	}

	// Test Read operation
	buf := make([]byte, 4096)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}
	if n == 0 {
		t.Error("Expected to read data, got 0 bytes")
	}
	if !bytes.Equal(buf[:n], testData[:n]) {
		t.Error("Read data does not match original data")
	}
}

// TestMmapReaderReadAt verifies random access read operations
func TestMmapReaderReadAt(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Create a large temporary file
	file, err := os.CreateTemp("", "mmap_test_readat_*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()

	// Write test data with identifiable pattern
	testData := bytes.Repeat([]byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ"), 5*1024*1024) // ~130MB
	if _, err := file.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Reopen for reading
	file.Close()
	file, err = os.Open(file.Name())
	if err != nil {
		t.Fatalf("Failed to reopen file: %v", err)
	}

	// Create MmapReader
	reader, err := NewMmapReader(file)
	if err != nil {
		t.Fatalf("Failed to create MmapReader: %v", err)
	}
	defer reader.Close()

	// Test ReadAt at different offsets
	tests := []struct {
		offset int64
		size   int
	}{
		{0, 26},                         // Beginning
		{1024 * 1024, 26},               // Middle
		{int64(len(testData)) - 26, 26}, // Near end
	}

	for _, tt := range tests {
		buf := make([]byte, tt.size)
		n, err := reader.ReadAt(buf, tt.offset)
		if err != nil && err != io.EOF {
			t.Errorf("ReadAt(offset=%d) failed: %v", tt.offset, err)
			continue
		}
		if !bytes.Equal(buf[:n], testData[tt.offset:tt.offset+int64(n)]) {
			t.Errorf("ReadAt(offset=%d) data mismatch", tt.offset)
		}
	}

	// Test negative offset
	buf := make([]byte, 10)
	_, err = reader.ReadAt(buf, -1)
	if err == nil {
		t.Error("Expected error for negative offset, got nil")
	}

	// Test offset beyond size
	_, err = reader.ReadAt(buf, reader.Size()+1)
	if err != io.EOF {
		t.Errorf("Expected EOF for offset beyond size, got %v", err)
	}
}

// TestMmapReaderSeek verifies seek operations
func TestMmapReaderSeek(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Create a large temporary file
	file, err := os.CreateTemp("", "mmap_test_seek_*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()

	// Write test data
	testData := bytes.Repeat([]byte("seek test data\n"), 9*1024*1024) // ~135MB
	if _, err := file.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Reopen for reading
	file.Close()
	file, err = os.Open(file.Name())
	if err != nil {
		t.Fatalf("Failed to reopen file: %v", err)
	}

	// Create MmapReader
	reader, err := NewMmapReader(file)
	if err != nil {
		t.Fatalf("Failed to create MmapReader: %v", err)
	}
	defer reader.Close()

	// Test SeekStart
	pos, err := reader.Seek(1024, io.SeekStart)
	if err != nil {
		t.Errorf("Seek(SeekStart) failed: %v", err)
	}
	if pos != 1024 {
		t.Errorf("Expected position 1024, got %d", pos)
	}

	// Test SeekCurrent
	pos, err = reader.Seek(512, io.SeekCurrent)
	if err != nil {
		t.Errorf("Seek(SeekCurrent) failed: %v", err)
	}
	if pos != 1536 {
		t.Errorf("Expected position 1536, got %d", pos)
	}

	// Test SeekEnd
	pos, err = reader.Seek(-100, io.SeekEnd)
	if err != nil {
		t.Errorf("Seek(SeekEnd) failed: %v", err)
	}
	if pos != reader.Size()-100 {
		t.Errorf("Expected position %d, got %d", reader.Size()-100, pos)
	}

	// Test invalid whence
	_, err = reader.Seek(0, 999)
	if err == nil {
		t.Error("Expected error for invalid whence, got nil")
	}

	// Test negative position
	_, err = reader.Seek(-1, io.SeekStart)
	if err == nil {
		t.Error("Expected error for negative position, got nil")
	}
}

// TestCopyWithMmap verifies mmap-optimized copy operations
func TestCopyWithMmap(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Create source file
	srcFile, err := os.CreateTemp("", "mmap_test_copy_src_*")
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	defer os.Remove(srcFile.Name())
	defer srcFile.Close()

	// Write test data above threshold
	testData := bytes.Repeat([]byte("copy test data\n"), 9*1024*1024) // ~135MB
	if _, err := srcFile.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Reopen source for reading
	srcFile.Close()
	srcFile, err = os.Open(srcFile.Name())
	if err != nil {
		t.Fatalf("Failed to reopen source file: %v", err)
	}

	// Create destination file
	dstFile, err := os.CreateTemp("", "mmap_test_copy_dst_*")
	if err != nil {
		t.Fatalf("Failed to create destination file: %v", err)
	}
	defer os.Remove(dstFile.Name())
	defer dstFile.Close()

	// Perform copy
	written, err := CopyWithMmap(dstFile, srcFile)
	if err != nil {
		t.Fatalf("CopyWithMmap failed: %v", err)
	}

	if written != int64(len(testData)) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), written)
	}

	// Verify data
	dstFile.Seek(0, io.SeekStart)
	dstData, err := io.ReadAll(dstFile)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if !bytes.Equal(dstData, testData) {
		t.Error("Data mismatch after CopyWithMmap")
	}
}

// TestCopyWithMmap_SmallFile verifies fallback for small files
func TestCopyWithMmap_SmallFile(t *testing.T) {
	// Create small source file (below threshold)
	srcFile, err := os.CreateTemp("", "mmap_test_small_src_*")
	if err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}
	defer os.Remove(srcFile.Name())
	defer srcFile.Close()

	// Write small amount of data
	testData := []byte("small file test data")
	if _, err := srcFile.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Reopen for reading
	srcFile.Close()
	srcFile, err = os.Open(srcFile.Name())
	if err != nil {
		t.Fatalf("Failed to reopen source file: %v", err)
	}

	// Create destination file
	dstFile, err := os.CreateTemp("", "mmap_test_small_dst_*")
	if err != nil {
		t.Fatalf("Failed to create destination file: %v", err)
	}
	defer os.Remove(dstFile.Name())
	defer dstFile.Close()

	// Copy should fall back to CopyOptimized
	written, err := CopyWithMmap(dstFile, srcFile)
	if err != nil {
		t.Fatalf("CopyWithMmap failed: %v", err)
	}

	if written != int64(len(testData)) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), written)
	}

	// Verify data
	dstFile.Seek(0, io.SeekStart)
	dstData, err := io.ReadAll(dstFile)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if !bytes.Equal(dstData, testData) {
		t.Error("Data mismatch after small file copy")
	}
}

// TestReadFileWithMmap verifies full file reading with mmap
func TestReadFileWithMmap(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Create large temporary file
	tmpFile, err := os.CreateTemp("", "mmap_test_readfile_*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	filename := tmpFile.Name()
	defer os.Remove(filename)

	// Write test data above threshold
	testData := bytes.Repeat([]byte("readfile test\n"), 10*1024*1024) // ~140MB
	if _, err := tmpFile.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}
	tmpFile.Close()

	// Read file with mmap
	data, err := ReadFileWithMmap(filename)
	if err != nil {
		t.Fatalf("ReadFileWithMmap failed: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Error("Data mismatch after ReadFileWithMmap")
	}
}

// TestReadFileWithMmap_SmallFile verifies fallback for small files
func TestReadFileWithMmap_SmallFile(t *testing.T) {
	// Create small temporary file
	tmpFile, err := os.CreateTemp("", "mmap_test_readfile_small_*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	filename := tmpFile.Name()
	defer os.Remove(filename)

	// Write small test data
	testData := []byte("small file for readfile test")
	if _, err := tmpFile.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}
	tmpFile.Close()

	// Read file (should fall back to os.ReadFile)
	data, err := ReadFileWithMmap(filename)
	if err != nil {
		t.Fatalf("ReadFileWithMmap failed: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Error("Data mismatch after small file ReadFileWithMmap")
	}
}

// TestMmapReader_EmptyFile verifies handling of empty files
func TestMmapReader_EmptyFile(t *testing.T) {
	// Create empty file
	file, err := os.CreateTemp("", "mmap_test_empty_*")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(file.Name())
	defer file.Close()

	// File is too small for mmap
	if MmapSupported(file) {
		t.Error("Expected empty file to not support mmap")
	}
}

// BenchmarkCopyWithMmap_LargeFile benchmarks mmap-optimized copy
func BenchmarkCopyWithMmap_LargeFile(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping large file benchmark in short mode")
	}

	// Create test data (256MB)
	testData := bytes.Repeat([]byte("X"), 256*1024*1024)

	b.ResetTimer()
	b.SetBytes(int64(len(testData)))

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		srcFile, _ := os.CreateTemp("", "bench_mmap_src_*")
		dstFile, _ := os.CreateTemp("", "bench_mmap_dst_*")
		srcFile.Write(testData)
		srcFile.Close()
		srcFile, _ = os.Open(srcFile.Name())
		b.StartTimer()

		CopyWithMmap(dstFile, srcFile)

		b.StopTimer()
		srcFile.Close()
		dstFile.Close()
		os.Remove(srcFile.Name())
		os.Remove(dstFile.Name())
	}
}

// BenchmarkCopyOptimized_LargeFile benchmarks standard optimized copy for comparison
func BenchmarkCopyOptimized_LargeFile(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping large file benchmark in short mode")
	}

	// Create test data (256MB)
	testData := bytes.Repeat([]byte("X"), 256*1024*1024)

	b.ResetTimer()
	b.SetBytes(int64(len(testData)))

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		srcFile, _ := os.CreateTemp("", "bench_opt_src_*")
		dstFile, _ := os.CreateTemp("", "bench_opt_dst_*")
		srcFile.Write(testData)
		srcFile.Seek(0, io.SeekStart)
		b.StartTimer()

		CopyOptimized(dstFile, srcFile)

		b.StopTimer()
		srcFile.Close()
		dstFile.Close()
		os.Remove(srcFile.Name())
		os.Remove(dstFile.Name())
	}
}

// BenchmarkReadFileWithMmap benchmarks full file reading with mmap
func BenchmarkReadFileWithMmap(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping large file benchmark in short mode")
	}

	// Create test file (256MB)
	tmpFile, _ := os.CreateTemp("", "bench_readfile_mmap_*")
	testData := bytes.Repeat([]byte("Y"), 256*1024*1024)
	tmpFile.Write(testData)
	filename := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(filename)

	b.ResetTimer()
	b.SetBytes(int64(len(testData)))

	for i := 0; i < b.N; i++ {
		_, _ = ReadFileWithMmap(filename)
	}
}
