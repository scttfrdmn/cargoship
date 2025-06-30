package compression

import (
	"io"
	"strings"
	"testing"
)

func TestNewCompressor(t *testing.T) {
	tests := []struct {
		name      string
		algorithm Algorithm
		level     Level
		wantErr   bool
	}{
		{"none algorithm", AlgorithmNone, LevelDefault, false},
		{"gzip algorithm", AlgorithmGzip, LevelDefault, false},
		{"zlib algorithm", AlgorithmZlib, LevelDefault, false},
		{"zstd algorithm", AlgorithmZstd, LevelDefault, false},
		{"s2 algorithm", AlgorithmS2, LevelDefault, false},
		{"lz4 algorithm", AlgorithmLZ4, LevelDefault, false},
		{"unsupported algorithm", Algorithm("unsupported"), LevelDefault, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comp, err := NewCompressor(tt.algorithm, tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewCompressor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if comp == nil {
					t.Errorf("NewCompressor() returned nil compressor")
					return
				}
				if comp.algorithm != tt.algorithm {
					t.Errorf("NewCompressor() algorithm = %v, want %v", comp.algorithm, tt.algorithm)
				}
				if comp.level != tt.level {
					t.Errorf("NewCompressor() level = %v, want %v", comp.level, tt.level)
				}
				if comp.blockSize != 64*1024 {
					t.Errorf("NewCompressor() blockSize = %v, want %v", comp.blockSize, 64*1024)
				}
			}
		})
	}
}

func TestCompressor_Compress_None(t *testing.T) {
	comp, err := NewCompressor(AlgorithmNone, LevelDefault)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	testData := "Hello, World! This is test data for compression."
	reader := strings.NewReader(testData)

	compressed, result, err := comp.Compress(reader)
	if err != nil {
		t.Errorf("Compress() error = %v", err)
		return
	}

	if result.Algorithm != AlgorithmNone {
		t.Errorf("Result algorithm = %v, want %v", result.Algorithm, AlgorithmNone)
	}
	if result.OriginalSize != int64(len(testData)) {
		t.Errorf("Result original size = %v, want %v", result.OriginalSize, len(testData))
	}
	if result.CompressedSize != int64(len(testData)) {
		t.Errorf("Result compressed size = %v, want %v", result.CompressedSize, len(testData))
	}

	// Verify data integrity
	compressedData, err := io.ReadAll(compressed)
	if err != nil {
		t.Errorf("ReadAll() error = %v", err)
	}
	if string(compressedData) != testData {
		t.Errorf("Compressed data = %v, want %v", string(compressedData), testData)
	}
}

func TestCompressor_Compress_Gzip(t *testing.T) {
	comp, err := NewCompressor(AlgorithmGzip, LevelDefault)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	testData := strings.Repeat("Hello, World! This is test data for compression. ", 100)
	reader := strings.NewReader(testData)

	compressed, result, err := comp.Compress(reader)
	if err != nil {
		t.Errorf("Compress() error = %v", err)
		return
	}

	if result.Algorithm != AlgorithmGzip {
		t.Errorf("Result algorithm = %v, want %v", result.Algorithm, AlgorithmGzip)
	}
	if result.OriginalSize != int64(len(testData)) {
		t.Errorf("Result original size = %v, want %v", result.OriginalSize, len(testData))
	}
	if result.CompressedSize >= int64(len(testData)) {
		t.Errorf("Compression should reduce size: compressed=%v >= original=%v", result.CompressedSize, len(testData))
	}
	if result.CompressionRatio <= 1.0 {
		t.Errorf("Compression ratio should be > 1.0, got %v", result.CompressionRatio)
	}

	// Test decompression
	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Errorf("Decompress() error = %v", err)
		return
	}

	decompressedData, err := io.ReadAll(decompressed)
	if err != nil {
		t.Errorf("ReadAll() error = %v", err)
	}
	if string(decompressedData) != testData {
		t.Errorf("Decompressed data doesn't match original")
	}
}

func TestCompressor_Compress_Zlib(t *testing.T) {
	comp, err := NewCompressor(AlgorithmZlib, LevelBest)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	testData := strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZ", 50)
	reader := strings.NewReader(testData)

	compressed, result, err := comp.Compress(reader)
	if err != nil {
		t.Errorf("Compress() error = %v", err)
		return
	}

	if result.Algorithm != AlgorithmZlib {
		t.Errorf("Result algorithm = %v, want %v", result.Algorithm, AlgorithmZlib)
	}

	// Test decompression
	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Errorf("Decompress() error = %v", err)
		return
	}

	decompressedData, err := io.ReadAll(decompressed)
	if err != nil {
		t.Errorf("ReadAll() error = %v", err)
	}
	if string(decompressedData) != testData {
		t.Errorf("Decompressed data doesn't match original")
	}
}

func TestCompressor_Compress_Zstd(t *testing.T) {
	levels := []Level{LevelFastest, LevelFast, LevelDefault, LevelBetter, LevelBest}
	
	for _, level := range levels {
		t.Run(string(rune(level)), func(t *testing.T) {
			comp, err := NewCompressor(AlgorithmZstd, level)
			if err != nil {
				t.Fatalf("NewCompressor() error = %v", err)
			}

			testData := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 100)
			reader := strings.NewReader(testData)

			compressed, result, err := comp.Compress(reader)
			if err != nil {
				t.Errorf("Compress() error = %v", err)
				return
			}

			if result.Algorithm != AlgorithmZstd {
				t.Errorf("Result algorithm = %v, want %v", result.Algorithm, AlgorithmZstd)
			}

			// Test decompression
			decompressed, err := comp.Decompress(compressed)
			if err != nil {
				t.Errorf("Decompress() error = %v", err)
				return
			}

			decompressedData, err := io.ReadAll(decompressed)
			if err != nil {
				t.Errorf("ReadAll() error = %v", err)
			}
			if string(decompressedData) != testData {
				t.Errorf("Decompressed data doesn't match original")
			}
		})
	}
}

func TestCompressor_Compress_S2(t *testing.T) {
	comp, err := NewCompressor(AlgorithmS2, LevelDefault)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	testData := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 80)
	reader := strings.NewReader(testData)

	compressed, result, err := comp.Compress(reader)
	if err != nil {
		t.Errorf("Compress() error = %v", err)
		return
	}

	if result.Algorithm != AlgorithmS2 {
		t.Errorf("Result algorithm = %v, want %v", result.Algorithm, AlgorithmS2)
	}

	// Test decompression
	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Errorf("Decompress() error = %v", err)
		return
	}

	decompressedData, err := io.ReadAll(decompressed)
	if err != nil {
		t.Errorf("ReadAll() error = %v", err)
	}
	if string(decompressedData) != testData {
		t.Errorf("Decompressed data doesn't match original")
	}
}

func TestCompressor_Compress_LZ4(t *testing.T) {
	comp, err := NewCompressor(AlgorithmLZ4, LevelFast)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	testData := strings.Repeat("1234567890", 1000)
	reader := strings.NewReader(testData)

	compressed, result, err := comp.Compress(reader)
	if err != nil {
		t.Errorf("Compress() error = %v", err)
		return
	}

	if result.Algorithm != AlgorithmLZ4 {
		t.Errorf("Result algorithm = %v, want %v", result.Algorithm, AlgorithmLZ4)
	}

	// Test decompression
	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Errorf("Decompress() error = %v", err)
		return
	}

	decompressedData, err := io.ReadAll(decompressed)
	if err != nil {
		t.Errorf("ReadAll() error = %v", err)
	}
	if string(decompressedData) != testData {
		t.Errorf("Decompressed data doesn't match original")
	}
}

func TestCompressor_UnsupportedAlgorithm(t *testing.T) {
	comp := &Compressor{algorithm: Algorithm("unsupported")}

	testData := "test data"
	reader := strings.NewReader(testData)

	_, _, err := comp.Compress(reader)
	if err == nil {
		t.Errorf("Compress() should fail with unsupported algorithm")
	}

	_, err = comp.Decompress(reader)
	if err == nil {
		t.Errorf("Decompress() should fail with unsupported algorithm")
	}
}

func TestGetSupportedAlgorithms(t *testing.T) {
	algorithms := GetSupportedAlgorithms()
	expected := []Algorithm{
		AlgorithmNone,
		AlgorithmGzip,
		AlgorithmZlib,
		AlgorithmZstd,
		AlgorithmLZ4,
		AlgorithmS2,
	}

	if len(algorithms) != len(expected) {
		t.Errorf("GetSupportedAlgorithms() length = %v, want %v", len(algorithms), len(expected))
	}

	for i, alg := range algorithms {
		if alg != expected[i] {
			t.Errorf("GetSupportedAlgorithms()[%v] = %v, want %v", i, alg, expected[i])
		}
	}
}

func TestBenchmarkCompression(t *testing.T) {
	testData := strings.Repeat("This is test data for compression benchmarking. ", 1000)
	reader := strings.NewReader(testData)

	results, err := BenchmarkCompression(reader, int64(len(testData)))
	if err != nil {
		t.Errorf("BenchmarkCompression() error = %v", err)
		return
	}

	if len(results) == 0 {
		t.Errorf("BenchmarkCompression() returned no results")
		return
	}

	// Verify each result has valid fields
	for _, result := range results {
		if result.Algorithm == "" {
			t.Errorf("Result missing algorithm")
		}
		if result.OriginalSize <= 0 {
			t.Errorf("Result has invalid original size: %v", result.OriginalSize)
		}
		if result.CompressedSize <= 0 {
			t.Errorf("Result has invalid compressed size: %v", result.CompressedSize)
		}
		if result.CompressionTime < 0 {
			t.Errorf("Result has invalid compression time: %v", result.CompressionTime)
		}
	}
}

func TestBenchmarkCompression_EmptyData(t *testing.T) {
	reader := strings.NewReader("")

	_, err := BenchmarkCompression(reader, 0)
	if err != nil {
		t.Errorf("BenchmarkCompression() should handle empty data, error = %v", err)
	}
}

func TestRecommendAlgorithm(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
		priority string
		expected Algorithm
	}{
		{"speed priority", "any", "speed", AlgorithmLZ4},
		{"size priority", "any", "size", AlgorithmZstd},
		{"balanced priority", "any", "balanced", AlgorithmS2},
		{"text data", "text", "", AlgorithmZstd},
		{"log data", "log", "", AlgorithmZstd},
		{"json data", "json", "", AlgorithmZstd},
		{"xml data", "xml", "", AlgorithmZstd},
		{"csv data", "csv", "", AlgorithmZstd},
		{"image data", "image", "", AlgorithmNone},
		{"video data", "video", "", AlgorithmNone},
		{"audio data", "audio", "", AlgorithmNone},
		{"binary data", "binary", "", AlgorithmS2},
		{"executable data", "executable", "", AlgorithmS2},
		{"database data", "database", "", AlgorithmZstd},
		{"backup data", "backup", "", AlgorithmZstd},
		{"unknown data", "unknown", "", AlgorithmZstd},
		{"empty data type", "", "", AlgorithmZstd},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RecommendAlgorithm(tt.dataType, tt.priority)
			if result != tt.expected {
				t.Errorf("RecommendAlgorithm() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCompressionResult_Fields(t *testing.T) {
	result := &CompressionResult{
		Algorithm:        AlgorithmGzip,
		Level:           LevelDefault,
		OriginalSize:    1000,
		CompressedSize:  800,
		CompressionRatio: 1.25,
		CompressionTime:  100,
		Throughput:      10.0,
	}

	if result.Algorithm != AlgorithmGzip {
		t.Errorf("Algorithm = %v, want %v", result.Algorithm, AlgorithmGzip)
	}
	if result.Level != LevelDefault {
		t.Errorf("Level = %v, want %v", result.Level, LevelDefault)
	}
	if result.OriginalSize != 1000 {
		t.Errorf("OriginalSize = %v, want 1000", result.OriginalSize)
	}
	if result.CompressedSize != 800 {
		t.Errorf("CompressedSize = %v, want 800", result.CompressedSize)
	}
	if result.CompressionRatio != 1.25 {
		t.Errorf("CompressionRatio = %v, want 1.25", result.CompressionRatio)
	}
	if result.CompressionTime != 100 {
		t.Errorf("CompressionTime = %v, want 100", result.CompressionTime)
	}
	if result.Throughput != 10.0 {
		t.Errorf("Throughput = %v, want 10.0", result.Throughput)
	}
}

func TestCompressor_PoolReuse(t *testing.T) {
	// Test that pools are properly reused
	comp, err := NewCompressor(AlgorithmGzip, LevelDefault)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	testData := "Test data for pool reuse"
	
	// Perform multiple compressions to test pool reuse
	for i := 0; i < 10; i++ {
		reader := strings.NewReader(testData)
		compressed, _, err := comp.Compress(reader)
		if err != nil {
			t.Errorf("Compress() iteration %v error = %v", i, err)
			continue
		}

		decompressed, err := comp.Decompress(compressed)
		if err != nil {
			t.Errorf("Decompress() iteration %v error = %v", i, err)
			continue
		}

		result, err := io.ReadAll(decompressed)
		if err != nil {
			t.Errorf("ReadAll() iteration %v error = %v", i, err)
			continue
		}

		if string(result) != testData {
			t.Errorf("Iteration %v: decompressed data doesn't match original", i)
		}
	}
}

func TestCompressor_LargeData(t *testing.T) {
	comp, err := NewCompressor(AlgorithmZstd, LevelFast)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	// Create larger test data (1MB)
	largeData := strings.Repeat("Large data for compression testing. ", 30000)
	reader := strings.NewReader(largeData)

	compressed, result, err := comp.Compress(reader)
	if err != nil {
		t.Errorf("Compress() error = %v", err)
		return
	}

	if result.OriginalSize != int64(len(largeData)) {
		t.Errorf("Original size = %v, want %v", result.OriginalSize, len(largeData))
	}

	// Test decompression
	decompressed, err := comp.Decompress(compressed)
	if err != nil {
		t.Errorf("Decompress() error = %v", err)
		return
	}

	decompressedData, err := io.ReadAll(decompressed)
	if err != nil {
		t.Errorf("ReadAll() error = %v", err)
		return
	}

	if len(decompressedData) != len(largeData) {
		t.Errorf("Decompressed size = %v, want %v", len(decompressedData), len(largeData))
	}

	if string(decompressedData) != largeData {
		t.Errorf("Decompressed data doesn't match original")
	}
}

func TestAlgorithmConstants(t *testing.T) {
	if AlgorithmNone != "none" {
		t.Errorf("AlgorithmNone = %v, want 'none'", AlgorithmNone)
	}
	if AlgorithmGzip != "gzip" {
		t.Errorf("AlgorithmGzip = %v, want 'gzip'", AlgorithmGzip)
	}
	if AlgorithmZlib != "zlib" {
		t.Errorf("AlgorithmZlib = %v, want 'zlib'", AlgorithmZlib)
	}
	if AlgorithmZstd != "zstd" {
		t.Errorf("AlgorithmZstd = %v, want 'zstd'", AlgorithmZstd)
	}
	if AlgorithmLZ4 != "lz4" {
		t.Errorf("AlgorithmLZ4 = %v, want 'lz4'", AlgorithmLZ4)
	}
	if AlgorithmS2 != "s2" {
		t.Errorf("AlgorithmS2 = %v, want 's2'", AlgorithmS2)
	}
}

func TestLevelConstants(t *testing.T) {
	if LevelFastest != 1 {
		t.Errorf("LevelFastest = %v, want 1", LevelFastest)
	}
	if LevelFast != 3 {
		t.Errorf("LevelFast = %v, want 3", LevelFast)
	}
	if LevelDefault != 5 {
		t.Errorf("LevelDefault = %v, want 5", LevelDefault)
	}
	if LevelBetter != 7 {
		t.Errorf("LevelBetter = %v, want 7", LevelBetter)
	}
	if LevelBest != 9 {
		t.Errorf("LevelBest = %v, want 9", LevelBest)
	}
}

func TestCompressor_ErrorHandling(t *testing.T) {
	// Test compression with read error
	comp, _ := NewCompressor(AlgorithmGzip, LevelDefault)
	
	errorReader := &errorReader{}
	_, _, err := comp.Compress(errorReader)
	if err == nil {
		t.Errorf("Compress() should fail with error reader")
	}
}

// errorReader is a helper that always returns an error
type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func TestCompressor_EmptyData(t *testing.T) {
	algorithms := []Algorithm{AlgorithmNone, AlgorithmGzip, AlgorithmZlib, AlgorithmZstd, AlgorithmS2, AlgorithmLZ4}
	
	for _, alg := range algorithms {
		t.Run(string(alg), func(t *testing.T) {
			comp, err := NewCompressor(alg, LevelDefault)
			if err != nil {
				t.Fatalf("NewCompressor() error = %v", err)
			}

			reader := strings.NewReader("")
			compressed, result, err := comp.Compress(reader)
			if err != nil {
				t.Errorf("Compress() error = %v", err)
				return
			}

			if result.OriginalSize != 0 {
				t.Errorf("Original size = %v, want 0", result.OriginalSize)
			}

			// Test decompression of empty data
			decompressed, err := comp.Decompress(compressed)
			if err != nil {
				t.Errorf("Decompress() error = %v", err)
				return
			}

			data, err := io.ReadAll(decompressed)
			if err != nil {
				t.Errorf("ReadAll() error = %v", err)
				return
			}

			if len(data) != 0 {
				t.Errorf("Decompressed empty data should be empty, got %v bytes", len(data))
			}
		})
	}
}

func TestBenchmarkCompression_ReadError(t *testing.T) {
	errorReader := &errorReader{}
	
	_, err := BenchmarkCompression(errorReader, 100)
	if err == nil {
		t.Errorf("BenchmarkCompression() should fail with error reader")
	}
}

func TestCompressor_CompressionStatistics(t *testing.T) {
	comp, err := NewCompressor(AlgorithmGzip, LevelBest)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	// Use highly compressible data
	testData := strings.Repeat("A", 10000)
	reader := strings.NewReader(testData)

	_, result, err := comp.Compress(reader)
	if err != nil {
		t.Errorf("Compress() error = %v", err)
		return
	}

	// Verify statistics are reasonable
	if result.OriginalSize != int64(len(testData)) {
		t.Errorf("OriginalSize = %v, want %v", result.OriginalSize, len(testData))
	}
	
	if result.CompressedSize >= result.OriginalSize {
		t.Errorf("CompressedSize (%v) should be less than OriginalSize (%v)", result.CompressedSize, result.OriginalSize)
	}
	
	if result.CompressionRatio <= 1.0 {
		t.Errorf("CompressionRatio should be > 1.0, got %v", result.CompressionRatio)
	}
	
	if result.CompressionTime < 0 {
		t.Errorf("CompressionTime should be >= 0, got %v", result.CompressionTime)
	}
	
	if result.Throughput < 0 {
		t.Errorf("Throughput should be >= 0, got %v", result.Throughput)
	}
}

func TestCompressor_ZeroCompressionRatio(t *testing.T) {
	comp, err := NewCompressor(AlgorithmNone, LevelDefault)
	if err != nil {
		t.Fatalf("NewCompressor() error = %v", err)
	}

	// Test with empty data that would result in zero compression ratio
	reader := strings.NewReader("")
	_, result, err := comp.Compress(reader)
	if err != nil {
		t.Errorf("Compress() error = %v", err)
		return
	}

	// For empty data, compression ratio calculation should handle division by zero
	// or the original and compressed sizes should both be 0
	if result.OriginalSize == 0 && result.CompressedSize == 0 {
		// This is expected for empty data
	} else if result.CompressedSize > 0 && result.OriginalSize == 0 {
		t.Errorf("Invalid state: compressed size > 0 but original size = 0")
	}
}