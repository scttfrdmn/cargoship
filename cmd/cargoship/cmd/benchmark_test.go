package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/scttfrdmn/cargoship/pkg/compression"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBenchmarkCmd(t *testing.T) {
	cmd := NewBenchmarkCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "benchmark", cmd.Use)
	assert.Equal(t, "Benchmark compression algorithms", cmd.Short)
	assert.NotEmpty(t, cmd.Long)
	assert.NotNil(t, cmd.RunE)

	// Test flags
	flags := cmd.Flags()
	assert.True(t, flags.HasFlags())
	
	sizeFlag := flags.Lookup("size")
	require.NotNil(t, sizeFlag)
	assert.Equal(t, "10MB", sizeFlag.DefValue)
	
	dataTypeFlag := flags.Lookup("data-type")
	require.NotNil(t, dataTypeFlag)
	assert.Equal(t, "mixed", dataTypeFlag.DefValue)
	
	formatFlag := flags.Lookup("format")
	require.NotNil(t, formatFlag)
	assert.Equal(t, "table", formatFlag.DefValue)
	
	fileFlag := flags.Lookup("file")
	require.NotNil(t, fileFlag)
	assert.Equal(t, "", fileFlag.DefValue)
}

func TestParseBytes(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		hasError bool
	}{
		{"1B", 1, false},
		{"1KB", 1024, false},
		{"1MB", 1024 * 1024, false},
		{"1GB", 1024 * 1024 * 1024, false},
		{"1TB", 1024 * 1024 * 1024 * 1024, false},
		{"2.5MB", int64(2.5 * 1024 * 1024), false},
		{"0.5GB", int64(0.5 * 1024 * 1024 * 1024), false},
		{"10mb", 10 * 1024 * 1024, false}, // Case insensitive
		{"invalid", 0, true},
		{"1XB", 0, true},
		{"", 0, true},
		{"MB", 0, true},
		{"1.5.5MB", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := parseBytes(tt.input)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 1024, "1.0 GB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TB"},
		{1024 * 1024 * 1024 * 1024 * 1024, "1.0 PB"},
		{2621440, "2.5 MB"},    // 2.5 * 1024 * 1024
		{1825361101, "1.7 GB"}, // 1.7 * 1024 * 1024 * 1024
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatBytes(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateTestData(t *testing.T) {
	testCases := []struct {
		dataType string
		size     int64
		hasError bool
	}{
		{"random", 1024, false},
		{"text", 1024, false},
		{"binary", 1024, false},
		{"mixed", 1024, false},
		{"invalid", 1024, true},
		{"text", 0, false}, // Edge case: empty data
	}

	for _, tc := range testCases {
		t.Run(tc.dataType, func(t *testing.T) {
			data, err := generateTestData(tc.size, tc.dataType)
			
			if tc.hasError {
				assert.Error(t, err)
				assert.Nil(t, data)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.size, int64(len(data)))
				
				// Test specific data type characteristics
				switch tc.dataType {
				case "random":
					if tc.size > 0 {
						// Random data should have some variation (check multiple bytes)
						// This is more robust than comparing just first and last byte
						allSame := true
						if tc.size > 1 {
							firstByte := data[0]
							for i := 1; i < len(data) && i < 10; i++ { // Check first 10 bytes
								if data[i] != firstByte {
									allSame = false
									break
								}
							}
							assert.False(t, allSame, "Random data should have variation in first 10 bytes")
						}
					}
				case "text":
					if tc.size > 0 {
						// Text data should contain recognizable patterns
						dataStr := string(data[:min(100, len(data))])
						assert.Contains(t, dataStr, "The quick brown fox")
					}
				case "binary":
					// Binary data should be deterministic
					if tc.size > 1100 {
						// Should have patterns every 1024 bytes
						assert.NotEqual(t, data[50], data[1074]) // Different patterns
					}
				case "mixed":
					if tc.size > 100 {
						// Mixed should contain text patterns
						dataStr := string(data[:min(100, len(data))])
						assert.Contains(t, dataStr, "Sample text content")
					}
				}
			}
		})
	}
}

func TestCalculateScore(t *testing.T) {
	tests := []struct {
		name     string
		result   compression.CompressionResult
		expected float64
	}{
		{
			name: "high compression low speed",
			result: compression.CompressionResult{
				CompressionRatio: 5.0,
				Throughput:       10.0,
			},
			expected: 5.0*10 + 10.0, // ratioScore + speedScore
		},
		{
			name: "low compression high speed",
			result: compression.CompressionResult{
				CompressionRatio: 2.0,
				Throughput:       50.0,
			},
			expected: 2.0*10 + 50.0,
		},
		{
			name: "zero values",
			result: compression.CompressionResult{
				CompressionRatio: 0.0,
				Throughput:       0.0,
			},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateScore(tt.result)
			assert.Equal(t, tt.expected, score)
		})
	}
}

func TestFindBestAlgorithm(t *testing.T) {
	tests := []struct {
		name     string
		results  []compression.CompressionResult
		expected compression.CompressionResult
	}{
		{
			name:     "empty results",
			results:  []compression.CompressionResult{},
			expected: compression.CompressionResult{},
		},
		{
			name: "single result",
			results: []compression.CompressionResult{
				{Algorithm: "gzip", Level: 6, CompressionRatio: 3.0, Throughput: 20.0},
			},
			expected: compression.CompressionResult{Algorithm: "gzip", Level: 6, CompressionRatio: 3.0, Throughput: 20.0},
		},
		{
			name: "multiple results - best by score",
			results: []compression.CompressionResult{
				{Algorithm: "gzip", Level: 6, CompressionRatio: 3.0, Throughput: 20.0}, // Score: 50
				{Algorithm: "zstd", Level: 3, CompressionRatio: 4.0, Throughput: 30.0}, // Score: 70
				{Algorithm: "lz4", Level: 1, CompressionRatio: 2.0, Throughput: 100.0}, // Score: 120
			},
			expected: compression.CompressionResult{Algorithm: "lz4", Level: 1, CompressionRatio: 2.0, Throughput: 100.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findBestAlgorithm(tt.results)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindFastestAlgorithm(t *testing.T) {
	tests := []struct {
		name     string
		results  []compression.CompressionResult
		expected compression.CompressionResult
	}{
		{
			name:     "empty results",
			results:  []compression.CompressionResult{},
			expected: compression.CompressionResult{},
		},
		{
			name: "single result",
			results: []compression.CompressionResult{
				{Algorithm: "gzip", Throughput: 20.0},
			},
			expected: compression.CompressionResult{Algorithm: "gzip", Throughput: 20.0},
		},
		{
			name: "multiple results",
			results: []compression.CompressionResult{
				{Algorithm: "gzip", Throughput: 20.0},
				{Algorithm: "zstd", Throughput: 30.0},
				{Algorithm: "lz4", Throughput: 100.0}, // Fastest
			},
			expected: compression.CompressionResult{Algorithm: "lz4", Throughput: 100.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findFastestAlgorithm(tt.results)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindBestRatioAlgorithm(t *testing.T) {
	tests := []struct {
		name     string
		results  []compression.CompressionResult
		expected compression.CompressionResult
	}{
		{
			name:     "empty results",
			results:  []compression.CompressionResult{},
			expected: compression.CompressionResult{},
		},
		{
			name: "single result",
			results: []compression.CompressionResult{
				{Algorithm: "gzip", CompressionRatio: 3.0},
			},
			expected: compression.CompressionResult{Algorithm: "gzip", CompressionRatio: 3.0},
		},
		{
			name: "multiple results",
			results: []compression.CompressionResult{
				{Algorithm: "lz4", CompressionRatio: 2.0},
				{Algorithm: "gzip", CompressionRatio: 3.0},
				{Algorithm: "zstd", CompressionRatio: 5.0}, // Best ratio
			},
			expected: compression.CompressionResult{Algorithm: "zstd", CompressionRatio: 5.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findBestRatioAlgorithm(tt.results)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOutputBenchmarkJSON(t *testing.T) {
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	results := []compression.CompressionResult{
		{Algorithm: "gzip", Level: 6, CompressionRatio: 3.0, Throughput: 20.0},
		{Algorithm: "zstd", Level: 3, CompressionRatio: 4.0, Throughput: 30.0},
	}

	err := outputBenchmarkJSON(results)
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout

	assert.NoError(t, err)

	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Verify JSON structure
	var parsed map[string]interface{}
	err = json.Unmarshal([]byte(output), &parsed)
	assert.NoError(t, err)

	// Verify required fields
	assert.Contains(t, parsed, "timestamp")
	assert.Contains(t, parsed, "results")
	assert.Contains(t, parsed, "recommendations")

	recommendations := parsed["recommendations"].(map[string]interface{})
	assert.Contains(t, recommendations, "best_overall")
	assert.Contains(t, recommendations, "fastest")
	assert.Contains(t, recommendations, "best_compression")
}

func TestRunBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping benchmark execution test in short mode")
	}
	
	t.Run("benchmark with flag-based input", func(t *testing.T) {
		cmd := NewBenchmarkCmd()
		
		// Set flags instead of global variables
		err := cmd.Flags().Set("size", "1KB")
		require.NoError(t, err)
		err = cmd.Flags().Set("data-type", "text")
		require.NoError(t, err)
		err = cmd.Flags().Set("format", "json")
		require.NoError(t, err)
		
		// Test execution (should complete without error for small data)
		err = cmd.RunE(cmd, []string{})
		assert.NoError(t, err)
	})
	
	t.Run("benchmark with file input via flags", func(t *testing.T) {
		// Create temporary test file
		tmpFile, err := os.CreateTemp("", "benchmark_test_*.txt")
		require.NoError(t, err)
		defer func() { _ = os.Remove(tmpFile.Name()) }()
		
		// Write test data
		testData := []byte("This is test data for benchmark testing. It should compress reasonably well.")
		err = os.WriteFile(tmpFile.Name(), testData, 0644)
		require.NoError(t, err)
		
		cmd := NewBenchmarkCmd()
		err = cmd.Flags().Set("file", tmpFile.Name())
		require.NoError(t, err)
		err = cmd.Flags().Set("format", "table")
		require.NoError(t, err)
		
		err = cmd.RunE(cmd, []string{})
		assert.NoError(t, err)
	})
	
	t.Run("non-existent file", func(t *testing.T) {
		cmd := NewBenchmarkCmd()
		err := cmd.Flags().Set("file", "/non/existent/file.txt")
		require.NoError(t, err)
		
		err = cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read file")
	})
	
	t.Run("invalid format", func(t *testing.T) {
		cmd := NewBenchmarkCmd()
		err := cmd.Flags().Set("size", "1KB")
		require.NoError(t, err)
		err = cmd.Flags().Set("format", "invalid")
		require.NoError(t, err)
		
		err = cmd.RunE(cmd, []string{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported format")
	})
}

func TestOutputBenchmarkTable(t *testing.T) {
	// Capture stdout
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	results := []compression.CompressionResult{
		{
			Algorithm:        "gzip",
			Level:           6,
			CompressionRatio: 3.0,
			CompressionTime:  100,
			CompressedSize:   1000,
		},
		{
			Algorithm:        "zstd", 
			Level:           3,
			CompressionRatio: 4.0,
			CompressionTime:  80,
			CompressedSize:   800,
		},
	}
	
	originalSize := int64(3000)
	err := outputBenchmarkTable(results, originalSize)
	
	// Restore stdout
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	// Verify table output contains expected content
	assert.Contains(t, output, "Compression Benchmark Results")
	assert.Contains(t, output, "Original size:")
	assert.Contains(t, output, "gzip")
	assert.Contains(t, output, "zstd")
	assert.Contains(t, output, "ALGORITHM")
	assert.Contains(t, output, "LEVEL")
	assert.Contains(t, output, "COMPRESSED SIZE")
	assert.Contains(t, output, "RATIO")
}

func TestOutputBenchmarkTableEmpty(t *testing.T) {
	// Test with empty results
	originalStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	
	results := []compression.CompressionResult{}
	err := outputBenchmarkTable(results, 1000)
	
	_ = w.Close()
	os.Stdout = originalStdout
	
	assert.NoError(t, err)
	
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()
	
	// Should still show headers and basic info
	assert.Contains(t, output, "Compression Benchmark Results")
	assert.Contains(t, output, "Original size:")
}

// These tests are disabled for now due to complexity with global state and compression package integration
// The utility functions (parseBytes, formatBytes, etc.) are well tested above and provide good coverage

// Helper function for compatibility
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}