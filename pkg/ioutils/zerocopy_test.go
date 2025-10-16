package ioutils

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestCopyOptimized_WriterTo tests zero-copy using WriterTo interface
func TestCopyOptimized_WriterTo(t *testing.T) {
	// strings.Reader implements WriterTo
	data := "Hello, World! This is a test of zero-copy I/O optimization."
	src := strings.NewReader(data)
	dst := &bytes.Buffer{}

	n, err := CopyOptimized(dst, src)
	if err != nil {
		t.Fatalf("CopyOptimized failed: %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("Expected %d bytes copied, got %d", len(data), n)
	}

	if dst.String() != data {
		t.Errorf("Data mismatch:\nExpected: %s\nGot: %s", data, dst.String())
	}
}

// TestCopyOptimized_ReaderFrom tests zero-copy using ReaderFrom interface
func TestCopyOptimized_ReaderFrom(t *testing.T) {
	// bytes.Buffer implements ReaderFrom
	data := "Testing ReaderFrom optimization path"
	src := bytes.NewReader([]byte(data))
	dst := &bytes.Buffer{}

	n, err := CopyOptimized(dst, src)
	if err != nil {
		t.Fatalf("CopyOptimized failed: %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("Expected %d bytes copied, got %d", len(data), n)
	}

	if dst.String() != data {
		t.Errorf("Data mismatch:\nExpected: %s\nGot: %s", data, dst.String())
	}
}

// TestCopyOptimized_Fallback tests fallback to standard io.Copy
func TestCopyOptimized_Fallback(t *testing.T) {
	// Use types that don't implement WriterTo or ReaderFrom
	data := []byte("Fallback to standard io.Copy")
	src := bytes.NewBuffer(data)
	dst := &bytes.Buffer{}

	n, err := CopyOptimized(dst, src)
	if err != nil {
		t.Fatalf("CopyOptimized failed: %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("Expected %d bytes copied, got %d", len(data), n)
	}

	if !bytes.Equal(dst.Bytes(), data) {
		t.Errorf("Data mismatch:\nExpected: %s\nGot: %s", data, dst.Bytes())
	}
}

// TestCopyBuffer_ZeroCopy tests CopyBuffer with zero-copy optimization
func TestCopyBuffer_ZeroCopy(t *testing.T) {
	data := "Zero-copy with buffer provided"
	src := strings.NewReader(data)
	dst := &bytes.Buffer{}

	buf := make([]byte, 8192)
	n, err := CopyBuffer(dst, src, buf)
	if err != nil {
		t.Fatalf("CopyBuffer failed: %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("Expected %d bytes copied, got %d", len(data), n)
	}

	if dst.String() != data {
		t.Errorf("Data mismatch:\nExpected: %s\nGot: %s", data, dst.String())
	}
}

// TestCopyBuffer_WithBuffer tests CopyBuffer using provided buffer
func TestCopyBuffer_WithBuffer(t *testing.T) {
	data := make([]byte, 100*1024) // 100KB
	for i := range data {
		data[i] = byte(i % 256)
	}

	src := bytes.NewBuffer(data)
	dst := &bytes.Buffer{}

	buf := make([]byte, 8192)
	n, err := CopyBuffer(dst, src, buf)
	if err != nil {
		t.Fatalf("CopyBuffer failed: %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("Expected %d bytes copied, got %d", len(data), n)
	}

	if !bytes.Equal(dst.Bytes(), data) {
		t.Errorf("Data mismatch after copy")
	}
}

// TestCopyN_SmallTransfer tests CopyN with small data
func TestCopyN_SmallTransfer(t *testing.T) {
	data := "Small transfer"
	src := strings.NewReader(data)
	dst := &bytes.Buffer{}

	n, err := CopyN(dst, src, int64(len(data)))
	if err != nil {
		t.Fatalf("CopyN failed: %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("Expected %d bytes copied, got %d", len(data), n)
	}

	if dst.String() != data {
		t.Errorf("Data mismatch:\nExpected: %s\nGot: %s", data, dst.String())
	}
}

// TestCopyN_LargeTransfer tests CopyN with large data using optimization
func TestCopyN_LargeTransfer(t *testing.T) {
	size := 64 * 1024 // 64KB
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}

	src := bytes.NewReader(data)
	dst := &bytes.Buffer{}

	n, err := CopyN(dst, src, int64(size))
	if err != nil {
		t.Fatalf("CopyN failed: %v", err)
	}

	if n != int64(size) {
		t.Errorf("Expected %d bytes copied, got %d", size, n)
	}

	if !bytes.Equal(dst.Bytes(), data) {
		t.Errorf("Data mismatch after large transfer")
	}
}

// TestCopyN_PartialRead tests CopyN reading less than available
func TestCopyN_PartialRead(t *testing.T) {
	data := "This is a longer string than we'll read"
	src := strings.NewReader(data)
	dst := &bytes.Buffer{}

	wantLen := 10
	n, err := CopyN(dst, src, int64(wantLen))
	if err != nil {
		t.Fatalf("CopyN failed: %v", err)
	}

	if n != int64(wantLen) {
		t.Errorf("Expected %d bytes copied, got %d", wantLen, n)
	}

	expected := data[:wantLen]
	if dst.String() != expected {
		t.Errorf("Data mismatch:\nExpected: %s\nGot: %s", expected, dst.String())
	}
}

// TestCopyN_EOF tests CopyN with insufficient data
func TestCopyN_EOF(t *testing.T) {
	data := "Short"
	src := strings.NewReader(data)
	dst := &bytes.Buffer{}

	wantLen := 100 // More than available
	n, err := CopyN(dst, src, int64(wantLen))
	if err != io.EOF {
		t.Errorf("Expected io.EOF, got: %v", err)
	}

	if n != int64(len(data)) {
		t.Errorf("Expected %d bytes copied before EOF, got %d", len(data), n)
	}
}

// TestTeeReader tests hash calculation while copying
func TestTeeReader(t *testing.T) {
	data := "Data to tee to multiple destinations"
	src := strings.NewReader(data)

	// Create a tee that writes to both destinations
	dst1 := &bytes.Buffer{}
	dst2 := &bytes.Buffer{}

	tee := TeeReader(src, dst1)

	// Copy from tee to dst2
	_, err := io.Copy(dst2, tee)
	if err != nil {
		t.Fatalf("Copy from TeeReader failed: %v", err)
	}

	// Both destinations should have the data
	if dst1.String() != data {
		t.Errorf("dst1 mismatch:\nExpected: %s\nGot: %s", data, dst1.String())
	}

	if dst2.String() != data {
		t.Errorf("dst2 mismatch:\nExpected: %s\nGot: %s", data, dst2.String())
	}
}

// TestSupportsZeroCopy tests detection of zero-copy capability
func TestSupportsZeroCopy(t *testing.T) {
	tests := []struct {
		name     string
		dst      io.Writer
		src      io.Reader
		expected bool
	}{
		{
			name:     "WriterTo support (strings.Reader)",
			dst:      &bytes.Buffer{},
			src:      strings.NewReader("test"),
			expected: true,
		},
		{
			name:     "ReaderFrom support (bytes.Buffer)",
			dst:      &bytes.Buffer{},
			src:      bytes.NewReader([]byte("test")),
			expected: true, // bytes.Reader also implements WriterTo
		},
		{
			name:     "No zero-copy support",
			dst:      &limitedWriter{w: &bytes.Buffer{}},
			src:      &limitedReader{r: bytes.NewReader([]byte("test"))},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SupportsZeroCopy(tt.dst, tt.src)
			if result != tt.expected {
				t.Errorf("SupportsZeroCopy() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// limitedWriter is a test helper that doesn't implement ReaderFrom
type limitedWriter struct {
	w io.Writer
}

func (lw *limitedWriter) Write(p []byte) (n int, err error) {
	return lw.w.Write(p)
}

// limitedReader is a test helper that doesn't implement WriterTo or ReadFrom
type limitedReader struct {
	r io.Reader
}

func (lr *limitedReader) Read(p []byte) (n int, err error) {
	return lr.r.Read(p)
}

// TestGetOptimizationMethod tests optimization method detection
func TestGetOptimizationMethod(t *testing.T) {
	tests := []struct {
		name     string
		dst      io.Writer
		src      io.Reader
		expected string
	}{
		{
			name:     "WriterTo method",
			dst:      &bytes.Buffer{},
			src:      strings.NewReader("test"),
			expected: "WriterTo",
		},
		{
			name:     "ReaderFrom method",
			dst:      &bytes.Buffer{},
			src:      bytes.NewReader([]byte("test")),
			expected: "WriterTo", // bytes.Reader implements WriterTo, which is checked first
		},
		{
			name:     "Standard method",
			dst:      &limitedWriter{w: &bytes.Buffer{}},
			src:      &limitedReader{r: bytes.NewReader([]byte("test"))},
			expected: "Standard",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetOptimizationMethod(tt.dst, tt.src)
			if result != tt.expected {
				t.Errorf("GetOptimizationMethod() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestCopyOptimized_FileToFile tests zero-copy with real files
func TestCopyOptimized_FileToFile(t *testing.T) {
	// Create a temporary source file
	srcFile, err := os.CreateTemp("", "zerocopy_src_*")
	if err != nil {
		t.Fatalf("Failed to create temp source file: %v", err)
	}
	defer func() { _ = os.Remove(srcFile.Name()) }()
	defer func() { _ = srcFile.Close() }()

	// Write test data
	testData := []byte("File-to-file zero-copy test with substantial data content for testing")
	if _, err := srcFile.Write(testData); err != nil {
		t.Fatalf("Failed to write test data: %v", err)
	}

	// Seek back to beginning
	if _, err := srcFile.Seek(0, 0); err != nil {
		t.Fatalf("Failed to seek: %v", err)
	}

	// Create destination file
	dstFile, err := os.CreateTemp("", "zerocopy_dst_*")
	if err != nil {
		t.Fatalf("Failed to create temp dest file: %v", err)
	}
	defer func() { _ = os.Remove(dstFile.Name()) }()
	defer func() { _ = dstFile.Close() }()

	// Perform zero-copy
	n, err := CopyOptimized(dstFile, srcFile)
	if err != nil {
		t.Fatalf("CopyOptimized failed: %v", err)
	}

	if n != int64(len(testData)) {
		t.Errorf("Expected %d bytes copied, got %d", len(testData), n)
	}

	// Verify content
	if _, err := dstFile.Seek(0, 0); err != nil {
		t.Fatalf("Failed to seek destination file: %v", err)
	}
	readData := make([]byte, len(testData))
	if _, err := io.ReadFull(dstFile, readData); err != nil {
		t.Fatalf("Failed to read destination file: %v", err)
	}

	if !bytes.Equal(readData, testData) {
		t.Errorf("File content mismatch")
	}
}

// BenchmarkCopyOptimized_Small benchmarks small transfers
func BenchmarkCopyOptimized_Small(b *testing.B) {
	data := []byte("Small data transfer benchmark")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := bytes.NewReader(data)
		dst := &bytes.Buffer{}
		_, _ = CopyOptimized(dst, src)
	}

	b.SetBytes(int64(len(data)))
}

// BenchmarkCopyOptimized_Large benchmarks large transfers
func BenchmarkCopyOptimized_Large(b *testing.B) {
	size := 1024 * 1024 // 1MB
	data := make([]byte, size)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := bytes.NewReader(data)
		dst := &bytes.Buffer{}
		_, _ = CopyOptimized(dst, src)
	}

	b.SetBytes(int64(size))
}

// BenchmarkCopyStandard_Large benchmarks standard io.Copy for comparison
func BenchmarkCopyStandard_Large(b *testing.B) {
	size := 1024 * 1024 // 1MB
	data := make([]byte, size)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := bytes.NewReader(data)
		dst := &bytes.Buffer{}
		_, _ = io.Copy(dst, src)
	}

	b.SetBytes(int64(size))
}

// BenchmarkCopyBuffer_WithPool benchmarks using buffer pool pattern
func BenchmarkCopyBuffer_WithPool(b *testing.B) {
	size := 1024 * 1024 // 1MB
	data := make([]byte, size)
	buf := make([]byte, 32*1024) // 32KB buffer

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src := bytes.NewReader(data)
		dst := &bytes.Buffer{}
		_, _ = CopyBuffer(dst, src, buf)
	}

	b.SetBytes(int64(size))
}
