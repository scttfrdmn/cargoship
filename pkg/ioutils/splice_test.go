package ioutils

import (
	"bytes"
	"io"
	"os"
	"runtime"
	"testing"
)

// TestSpliceSupported verifies platform-specific splice support detection
func TestSpliceSupported(t *testing.T) {
	// Create temporary files for testing
	srcFile, err := os.CreateTemp("", "splice_test_src_*")
	if err != nil {
		t.Fatalf("Failed to create temp source file: %v", err)
	}
	defer os.Remove(srcFile.Name())
	defer srcFile.Close()

	dstFile, err := os.CreateTemp("", "splice_test_dst_*")
	if err != nil {
		t.Fatalf("Failed to create temp destination file: %v", err)
	}
	defer os.Remove(dstFile.Name())
	defer dstFile.Close()

	// Write test data
	testData := bytes.Repeat([]byte("test data for splice operation\n"), 100)
	if _, err := srcFile.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}
	if _, err := srcFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek to start: %v", err)
	}

	// Test splice support detection
	isSupported := SpliceSupported(dstFile, srcFile)

	// On Linux, splice should be supported for regular files
	// On other platforms, it should not be supported
	if runtime.GOOS == "linux" {
		if !isSupported {
			t.Error("Expected splice to be supported on Linux for regular files")
		}
	} else {
		if isSupported {
			t.Error("Expected splice to NOT be supported on non-Linux platforms")
		}
	}
}

// TestSpliceSupportedWithNonFiles verifies that splice correctly rejects non-file types
func TestSpliceSupportedWithNonFiles(t *testing.T) {
	// Test with bytes.Buffer (not a file)
	src := bytes.NewReader([]byte("test data"))
	dst := &bytes.Buffer{}

	isSupported := SpliceSupported(dst, src)
	if isSupported {
		t.Error("Expected splice to NOT be supported for bytes.Buffer")
	}
}

// TestCopyOptimizedWithSplice_Fallback verifies graceful fallback on all platforms
func TestCopyOptimizedWithSplice_Fallback(t *testing.T) {
	testData := bytes.Repeat([]byte("test data for splice fallback\n"), 50)
	src := bytes.NewReader(testData)
	dst := &bytes.Buffer{}

	written, err := CopyOptimizedWithSplice(dst, src)
	if err != nil {
		t.Fatalf("CopyOptimizedWithSplice failed: %v", err)
	}

	if written != int64(len(testData)) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), written)
	}

	if !bytes.Equal(dst.Bytes(), testData) {
		t.Error("Data mismatch after CopyOptimizedWithSplice")
	}
}

// TestCopyOptimizedWithSplice_Files verifies splice with actual files
func TestCopyOptimizedWithSplice_Files(t *testing.T) {
	// Create temporary files
	srcFile, err := os.CreateTemp("", "splice_test_src_*")
	if err != nil {
		t.Fatalf("Failed to create temp source file: %v", err)
	}
	defer os.Remove(srcFile.Name())
	defer srcFile.Close()

	dstFile, err := os.CreateTemp("", "splice_test_dst_*")
	if err != nil {
		t.Fatalf("Failed to create temp destination file: %v", err)
	}
	defer os.Remove(dstFile.Name())
	defer dstFile.Close()

	// Write test data
	testData := bytes.Repeat([]byte("test data for file splice operation\n"), 100)
	if _, err := srcFile.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}
	if _, err := srcFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek to start: %v", err)
	}

	// Perform copy
	written, err := CopyOptimizedWithSplice(dstFile, srcFile)
	if err != nil {
		t.Fatalf("CopyOptimizedWithSplice failed: %v", err)
	}

	if written != int64(len(testData)) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), written)
	}

	// Verify data
	if _, err := dstFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek destination to start: %v", err)
	}

	dstData, err := io.ReadAll(dstFile)
	if err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if !bytes.Equal(dstData, testData) {
		t.Error("Data mismatch after file copy with splice")
	}
}

// TestCopySplice_UnsupportedTypes verifies error handling for unsupported types
func TestCopySplice_UnsupportedTypes(t *testing.T) {
	src := bytes.NewReader([]byte("test data"))
	dst := &bytes.Buffer{}

	written, err := CopySplice(dst, src)
	if err == nil {
		t.Error("Expected error when using CopySplice with unsupported types")
	}

	if err != ErrSpliceUnsupported {
		t.Errorf("Expected ErrSpliceUnsupported, got %v", err)
	}

	if written != 0 {
		t.Errorf("Expected 0 bytes written on error, got %d", written)
	}
}

// TestCopyOptimizedWithSplice_LargeFile verifies splice with large file
func TestCopyOptimizedWithSplice_LargeFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large file test in short mode")
	}

	// Create temporary files
	srcFile, err := os.CreateTemp("", "splice_test_large_src_*")
	if err != nil {
		t.Fatalf("Failed to create temp source file: %v", err)
	}
	defer os.Remove(srcFile.Name())
	defer srcFile.Close()

	dstFile, err := os.CreateTemp("", "splice_test_large_dst_*")
	if err != nil {
		t.Fatalf("Failed to create temp destination file: %v", err)
	}
	defer os.Remove(dstFile.Name())
	defer dstFile.Close()

	// Write 32MB of test data (larger than spliceChunkSize)
	chunkSize := 1024 * 1024 // 1MB chunks
	totalChunks := 32
	testChunk := bytes.Repeat([]byte("X"), chunkSize)

	for i := 0; i < totalChunks; i++ {
		if _, err := srcFile.Write(testChunk); err != nil {
			t.Fatalf("Failed to write test chunk %d: %v", i, err)
		}
	}

	expectedSize := int64(chunkSize * totalChunks)

	if _, err := srcFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Failed to seek to start: %v", err)
	}

	// Perform copy
	written, err := CopyOptimizedWithSplice(dstFile, srcFile)
	if err != nil {
		t.Fatalf("CopyOptimizedWithSplice failed: %v", err)
	}

	if written != expectedSize {
		t.Errorf("Expected to write %d bytes, wrote %d", expectedSize, written)
	}

	// Verify file size
	dstInfo, err := dstFile.Stat()
	if err != nil {
		t.Fatalf("Failed to stat destination file: %v", err)
	}

	if dstInfo.Size() != expectedSize {
		t.Errorf("Expected destination file size %d, got %d", expectedSize, dstInfo.Size())
	}
}

// TestCopyOptimizedWithSplice_EmptyFile verifies handling of empty files
func TestCopyOptimizedWithSplice_EmptyFile(t *testing.T) {
	// Create temporary files
	srcFile, err := os.CreateTemp("", "splice_test_empty_src_*")
	if err != nil {
		t.Fatalf("Failed to create temp source file: %v", err)
	}
	defer os.Remove(srcFile.Name())
	defer srcFile.Close()

	dstFile, err := os.CreateTemp("", "splice_test_empty_dst_*")
	if err != nil {
		t.Fatalf("Failed to create temp destination file: %v", err)
	}
	defer os.Remove(dstFile.Name())
	defer dstFile.Close()

	// Don't write any data - test with empty file

	// Perform copy
	written, err := CopyOptimizedWithSplice(dstFile, srcFile)
	if err != nil {
		t.Fatalf("CopyOptimizedWithSplice failed on empty file: %v", err)
	}

	if written != 0 {
		t.Errorf("Expected to write 0 bytes for empty file, wrote %d", written)
	}
}

// BenchmarkCopyOptimizedWithSplice_SmallFile benchmarks splice with small files (1MB)
func BenchmarkCopyOptimizedWithSplice_SmallFile(b *testing.B) {
	testData := bytes.Repeat([]byte("benchmark data\n"), 64*1024) // ~1MB

	b.ResetTimer()
	b.SetBytes(int64(len(testData)))

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		srcFile, _ := os.CreateTemp("", "bench_src_*")
		dstFile, _ := os.CreateTemp("", "bench_dst_*")
		srcFile.Write(testData)
		srcFile.Seek(0, io.SeekStart)
		b.StartTimer()

		CopyOptimizedWithSplice(dstFile, srcFile)

		b.StopTimer()
		srcFile.Close()
		dstFile.Close()
		os.Remove(srcFile.Name())
		os.Remove(dstFile.Name())
	}
}

// BenchmarkCopyOptimized_SmallFile benchmarks standard CopyOptimized for comparison
func BenchmarkCopyOptimized_SmallFile(b *testing.B) {
	testData := bytes.Repeat([]byte("benchmark data\n"), 64*1024) // ~1MB

	b.ResetTimer()
	b.SetBytes(int64(len(testData)))

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		srcFile, _ := os.CreateTemp("", "bench_src_*")
		dstFile, _ := os.CreateTemp("", "bench_dst_*")
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

// BenchmarkCopyOptimizedWithSplice_LargeFile benchmarks splice with large files (16MB)
func BenchmarkCopyOptimizedWithSplice_LargeFile(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping large file benchmark in short mode")
	}

	testData := bytes.Repeat([]byte("X"), 16*1024*1024) // 16MB

	b.ResetTimer()
	b.SetBytes(int64(len(testData)))

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		srcFile, _ := os.CreateTemp("", "bench_large_src_*")
		dstFile, _ := os.CreateTemp("", "bench_large_dst_*")
		srcFile.Write(testData)
		srcFile.Seek(0, io.SeekStart)
		b.StartTimer()

		CopyOptimizedWithSplice(dstFile, srcFile)

		b.StopTimer()
		srcFile.Close()
		dstFile.Close()
		os.Remove(srcFile.Name())
		os.Remove(dstFile.Name())
	}
}
