package chunking

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewArchivePadder(t *testing.T) {
	padder := NewArchivePadder()
	require.NotNil(t, padder)
	assert.True(t, padder.useLowEntropyPadding, "Should default to low-entropy (zero-byte) padding")
}

func TestNewArchivePadderWithConfig(t *testing.T) {
	// Test with low-entropy padding
	padderLow := NewArchivePadderWithConfig(true)
	require.NotNil(t, padderLow)
	assert.True(t, padderLow.useLowEntropyPadding)

	// Test with high-entropy padding
	padderHigh := NewArchivePadderWithConfig(false)
	require.NotNil(t, padderHigh)
	assert.False(t, padderHigh.useLowEntropyPadding)
}

func TestArchivePadder_PadToTarget_AlreadyAtTarget(t *testing.T) {
	padder := NewArchivePadder()
	var buf bytes.Buffer

	// Already at target, no padding needed
	paddingWritten, err := padder.PadToTarget(&buf, 100, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), paddingWritten)
	assert.Equal(t, 0, buf.Len())
}

func TestArchivePadder_PadToTarget_AboveTarget(t *testing.T) {
	padder := NewArchivePadder()
	var buf bytes.Buffer

	// Above target, no padding needed
	paddingWritten, err := padder.PadToTarget(&buf, 150, 100)
	require.NoError(t, err)
	assert.Equal(t, int64(0), paddingWritten)
	assert.Equal(t, 0, buf.Len())
}

func TestArchivePadder_PadToTarget_LowEntropy_SmallPadding(t *testing.T) {
	padder := NewArchivePadderWithConfig(true) // Low-entropy (zero bytes)
	var buf bytes.Buffer

	// Write some initial data
	initialData := []byte("test data")
	buf.Write(initialData)

	currentSize := int64(buf.Len())
	targetSize := currentSize + 100 // Need 100 bytes padding

	paddingWritten, err := padder.PadToTarget(&buf, currentSize, targetSize)
	require.NoError(t, err)
	assert.Equal(t, int64(100), paddingWritten)
	assert.Equal(t, int(targetSize), buf.Len())

	// Verify padding is all zeros
	data := buf.Bytes()
	for i := len(initialData); i < len(data); i++ {
		assert.Equal(t, byte(0), data[i], "Padding should be zero bytes")
	}
}

func TestArchivePadder_PadToTarget_LowEntropy_LargePadding(t *testing.T) {
	padder := NewArchivePadderWithConfig(true)
	var buf bytes.Buffer

	currentSize := int64(1000)
	targetSize := int64(10 * 1024 * 1024) // 10MB target

	paddingWritten, err := padder.PadToTarget(&buf, currentSize, targetSize)
	require.NoError(t, err)
	assert.Equal(t, targetSize-currentSize, paddingWritten)
	assert.Equal(t, int(targetSize-currentSize), buf.Len())

	// Verify a sample of padding is zeros (checking all 10MB would be slow)
	data := buf.Bytes()
	for i := 0; i < 1000; i++ {
		assert.Equal(t, byte(0), data[i], "Padding should be zero bytes")
	}
}

func TestArchivePadder_PadToTarget_HighEntropy_SmallPadding(t *testing.T) {
	padder := NewArchivePadderWithConfig(false) // High-entropy
	var buf bytes.Buffer

	currentSize := int64(0)
	targetSize := int64(100)

	paddingWritten, err := padder.PadToTarget(&buf, currentSize, targetSize)
	require.NoError(t, err)
	assert.Equal(t, int64(100), paddingWritten)
	assert.Equal(t, 100, buf.Len())

	// Verify padding is NOT all zeros (high entropy)
	data := buf.Bytes()
	allZeros := true
	for i := 0; i < len(data); i++ {
		if data[i] != 0 {
			allZeros = false
			break
		}
	}
	assert.False(t, allZeros, "High-entropy padding should not be all zeros")
}

func TestArchivePadder_CalculatePaddingSize(t *testing.T) {
	padder := NewArchivePadder()

	tests := []struct {
		name        string
		currentSize int64
		targetSize  int64
		expected    int64
	}{
		{"No padding needed (at target)", 100, 100, 0},
		{"No padding needed (above target)", 150, 100, 0},
		{"Small padding", 50, 100, 50},
		{"Large padding", 1024, 10*1024*1024, 10*1024*1024 - 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := padder.CalculatePaddingSize(tt.currentSize, tt.targetSize)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestArchivePadder_PadToTargetWithInfo(t *testing.T) {
	padder := NewArchivePadder()
	var buf bytes.Buffer

	currentSize := int64(1000)
	targetSize := int64(2000)

	info, err := padder.PadToTargetWithInfo(&buf, currentSize, targetSize)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, currentSize, info.OriginalSize)
	assert.Equal(t, targetSize, info.TargetSize)
	assert.Equal(t, int64(1000), info.PaddingBytes)
	assert.Equal(t, int64(2000), info.FinalSize)
	assert.Equal(t, 0.5, info.PaddingRatio) // 1000/2000 = 0.5
	assert.True(t, info.LowEntropyPadding)
}

func TestValidatePaddingRatio_Acceptable(t *testing.T) {
	info := &PaddingInfo{
		OriginalSize:     8000,
		TargetSize:       10000,
		PaddingBytes:     2000,
		FinalSize:        10000,
		PaddingRatio:     0.2, // 20% padding
		LowEntropyPadding: true,
	}

	isValid, message := ValidatePaddingRatio(info, 0.25) // Max 25%
	assert.True(t, isValid)
	assert.Contains(t, message, "acceptable")
	assert.Contains(t, message, "20.00%")
}

func TestValidatePaddingRatio_Excessive(t *testing.T) {
	info := &PaddingInfo{
		OriginalSize:      3000,
		TargetSize:        10000,
		PaddingBytes:      7000,
		FinalSize:         10000,
		PaddingRatio:      0.7, // 70% padding
		LowEntropyPadding: true,
	}

	isValid, message := ValidatePaddingRatio(info, 0.25) // Max 25%
	assert.False(t, isValid)
	assert.Contains(t, message, "exceeds maximum")
	assert.Contains(t, message, "70.00%")
}

func TestArchivePadder_PadToTarget_ChunkBoundary(t *testing.T) {
	// Test padding across 1MB chunk boundaries
	padder := NewArchivePadder()
	var buf bytes.Buffer

	currentSize := int64(512 * 1024)        // 512KB
	targetSize := int64(2*1024*1024 + 100) // 2MB + 100 bytes

	paddingWritten, err := padder.PadToTarget(&buf, currentSize, targetSize)
	require.NoError(t, err)
	assert.Equal(t, targetSize-currentSize, paddingWritten)
	assert.Equal(t, int(targetSize-currentSize), buf.Len())
}

func TestArchivePadder_PadToTarget_ExactMultipleMB(t *testing.T) {
	// Test padding to exact multiple of MB
	padder := NewArchivePadder()
	var buf bytes.Buffer

	currentSize := int64(100)
	targetSize := int64(5 * 1024 * 1024) // Exactly 5MB

	paddingWritten, err := padder.PadToTarget(&buf, currentSize, targetSize)
	require.NoError(t, err)
	assert.Equal(t, targetSize-currentSize, paddingWritten)
	assert.Equal(t, int(targetSize-currentSize), buf.Len())
}

func TestArchivePadder_Compressibility(t *testing.T) {
	// Demonstrate that low-entropy padding compresses well
	padderLow := NewArchivePadderWithConfig(true)
	padderHigh := NewArchivePadderWithConfig(false)

	var bufLow, bufHigh bytes.Buffer

	currentSize := int64(0)
	targetSize := int64(10 * 1024) // 10KB

	// Pad with low entropy
	_, err := padderLow.PadToTarget(&bufLow, currentSize, targetSize)
	require.NoError(t, err)

	// Pad with high entropy
	_, err = padderHigh.PadToTarget(&bufHigh, currentSize, targetSize)
	require.NoError(t, err)

	// Low-entropy padding should be more compressible
	// (All zeros compress extremely well)
	lowEntropyData := bufLow.Bytes()
	highEntropyData := bufHigh.Bytes()

	// Check that low-entropy is actually zeros
	allZeros := true
	for _, b := range lowEntropyData {
		if b != 0 {
			allZeros = false
			break
		}
	}
	assert.True(t, allZeros, "Low-entropy padding should be all zeros")

	// Check that high-entropy has variety
	uniqueBytes := make(map[byte]bool)
	for _, b := range highEntropyData {
		uniqueBytes[b] = true
	}
	assert.Greater(t, len(uniqueBytes), 10, "High-entropy padding should have variety")

	t.Logf("Low-entropy padding: All zeros (highly compressible)")
	t.Logf("High-entropy padding: %d unique byte values", len(uniqueBytes))
}

func TestArchivePadder_PadToTargetWithInfo_ZeroPadding(t *testing.T) {
	padder := NewArchivePadder()
	var buf bytes.Buffer

	// Already at target
	info, err := padder.PadToTargetWithInfo(&buf, 100, 100)
	require.NoError(t, err)
	require.NotNil(t, info)

	assert.Equal(t, int64(100), info.OriginalSize)
	assert.Equal(t, int64(100), info.TargetSize)
	assert.Equal(t, int64(0), info.PaddingBytes)
	assert.Equal(t, int64(100), info.FinalSize)
	assert.Equal(t, 0.0, info.PaddingRatio)
}

// Benchmark tests
func BenchmarkArchivePadder_LowEntropy_1MB(b *testing.B) {
	padder := NewArchivePadderWithConfig(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_, err := padder.PadToTarget(&buf, 0, 1024*1024)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkArchivePadder_HighEntropy_1MB(b *testing.B) {
	padder := NewArchivePadderWithConfig(false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_, err := padder.PadToTarget(&buf, 0, 1024*1024)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkArchivePadder_LowEntropy_10MB(b *testing.B) {
	padder := NewArchivePadderWithConfig(true)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var buf bytes.Buffer
		_, err := padder.PadToTarget(&buf, 0, 10*1024*1024)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Helper function for testing
var _ = createTestData // Prevent unused warning

func createTestData(size int) []byte {
	return []byte(strings.Repeat("x", size))
}
