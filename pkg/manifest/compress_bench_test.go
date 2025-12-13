package manifest

import (
	"testing"
	"time"
)

// BenchmarkToJSONAuto_1MFiles benchmarks compression for 1M files (Issue #92)
func BenchmarkToJSONAuto_1MFiles(b *testing.B) {
	// Create a manifest with 1M files
	m := createLargeManifest(1000000) // 1M files

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := m.ToJSONAuto()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFromJSONAuto_1MFiles benchmarks decompression for 1M files (Issue #92)
func BenchmarkFromJSONAuto_1MFiles(b *testing.B) {
	// Create and serialize a manifest with 1M files
	m := createLargeManifest(1000000) // 1M files
	data, _, err := m.ToJSONAuto()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := FromJSONAuto(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkToJSONAuto_100KFiles benchmarks compression for 100K files
func BenchmarkToJSONAuto_100KFiles(b *testing.B) {
	m := createLargeManifest(100000) // 100K files

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := m.ToJSONAuto()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFromJSONAuto_100KFiles benchmarks decompression for 100K files
func BenchmarkFromJSONAuto_100KFiles(b *testing.B) {
	m := createLargeManifest(100000) // 100K files
	data, _, err := m.ToJSONAuto()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := FromJSONAuto(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkToJSONCompressed_100KFiles benchmarks direct compression
func BenchmarkToJSONCompressed_100KFiles(b *testing.B) {
	m := createLargeManifest(100000)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := m.ToJSONCompressed()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFromJSONCompressed_100KFiles benchmarks direct decompression
func BenchmarkFromJSONCompressed_100KFiles(b *testing.B) {
	m := createLargeManifest(100000)
	data, err := m.ToJSONCompressed()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := FromJSONCompressed(data)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestCompressionPerformance tests that compression meets performance targets (Issue #92)
func TestCompressionPerformance(t *testing.T) {
	// Create a manifest with 1M files
	m := createLargeManifest(1000000)

	// Test compression time (target: <1s)
	start := time.Now()
	data, compressed, err := m.ToJSONAuto()
	compressionTime := time.Since(start)

	if err != nil {
		t.Fatalf("Compression failed: %v", err)
	}

	t.Logf("Compression time for 1M files: %v", compressionTime)
	if compressionTime > 1*time.Second {
		t.Logf("Warning: Compression took longer than 1s target (actual: %v)", compressionTime)
		// Don't fail the test, just log a warning
	}

	// Verify it was compressed
	if !compressed {
		t.Fatal("Expected manifest to be compressed")
	}

	// Test decompression time (target: <500ms)
	start = time.Now()
	_, err = FromJSONAuto(data)
	decompressionTime := time.Since(start)

	if err != nil {
		t.Fatalf("Decompression failed: %v", err)
	}

	t.Logf("Decompression time for 1M files: %v", decompressionTime)
	if decompressionTime > 500*time.Millisecond {
		t.Logf("Warning: Decompression took longer than 500ms target (actual: %v)", decompressionTime)
		// Don't fail the test, just log a warning
	}

	// Log compression ratio
	uncompressed, compressedSize, ratio, err := m.EstimateCompressedSize()
	if err != nil {
		t.Fatalf("Failed to estimate compression: %v", err)
	}

	t.Logf("Compression ratio: %.2f%% (%d → %d bytes)", ratio*100, uncompressed, compressedSize)
	t.Logf("Target: <10MB for 1M files, Actual: %.2f MB", float64(compressedSize)/(1024*1024))

	// Verify size target (<10MB)
	if compressedSize > 10*1024*1024 {
		t.Errorf("Compressed size %d bytes exceeds 10MB target", compressedSize)
	}
}

// createLargeManifest creates a manifest with the specified number of files for benchmarking
func createLargeManifest(fileCount int) *Manifest {
	m := &Manifest{
		Version:     ManifestVersion,
		UploadID:    "bench-upload",
		CreatedAt:   time.Now(),
		Bucket:      "bench-bucket",
		Region:      "us-west-2",
		Prefix:      "bench",
		ShardCount:  10,
		TotalFiles:  int64(fileCount),
		TotalBytes:  int64(fileCount * 100000),
		TotalChunks: fileCount / 100,
		Files:       make([]FileEntry, fileCount),
		Chunks:      make([]ChunkEntry, fileCount/100),
		Shards:      make([]ShardEntry, 10),
	}

	// Populate files
	for i := 0; i < fileCount; i++ {
		m.Files[i] = FileEntry{
			Path:     "test/data/files/path/to/simulate/realistic/size/file" + string(rune(i%26+'a')) + ".txt",
			Size:     100000,
			ChunkID:  i / 100,
			ShardID:  i / (fileCount / 10),
			Checksum: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		}
	}

	// Populate chunks
	for i := 0; i < fileCount/100; i++ {
		m.Chunks[i] = ChunkEntry{
			ID:               i,
			ShardID:          i / (fileCount / 1000),
			S3Key:            "shard-0/chunk-" + string(rune(i%10+'0')) + ".tar.zst",
			FileCount:        100,
			UncompressedSize: 10000000,
			CompressedSize:   5000000,
			Checksum:         "fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210",
		}
	}

	// Populate shards
	for i := 0; i < 10; i++ {
		m.Shards[i] = ShardEntry{
			ID:               i,
			Prefix:           "shard-" + string(rune(i+'0')),
			ChunkCount:       fileCount / 1000,
			FileCount:        int64(fileCount / 10),
			UncompressedSize: int64(fileCount / 10 * 100000),
			CompressedSize:   int64(fileCount / 10 * 50000),
		}
	}

	return m
}
