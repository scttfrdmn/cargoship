package cmd

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	porter "github.com/scttfrdmn/cargoship/pkg"
)

func BenchmarkGetSha256(b *testing.B) {
	tf := os.Getenv("BENCHMARK_SHA256_FILE")
	if tf == "" {
		tf = "../../../pkg/testdata/fakey-private.key"
	}
	for _, ha := range []string{"sha256", "sha512", "md5", "sha1"} {
		b.Run(fmt.Sprintf("suitcase_%v_%v", ha, tf), func(b *testing.B) {
			tfh, err := os.Open(tf)
			require.NoError(b, err)
			defer dclose(tfh)
			got := porter.MustCalculateHash(tfh, ha)
			require.NotEmpty(b, got)
		})
	}
}

func TestGetSha256(t *testing.T) {
	got, err := getSha256("../../../pkg/testdata/fakey-private.key")
	require.NoError(t, err)
	require.Equal(t, "07a115fe11a34882f3b54bee5a5ac2262405e33bf066b8d70b281fa2b4c01edb", got)
}

func BenchmarkCalculateHashes(b *testing.B) {
	bdd := os.Getenv("BENCHMARK_DATA_DIR")
	if bdd == "" {
		bdd = "../../../benchmark_data/"
	}

	var allFiles []string
	err := filepath.Walk(bdd, func(p string, info os.FileInfo, _ error) error {
		if !info.IsDir() {
			allFiles = append(allFiles, p)
		}
		return nil
	})
	require.NoError(b, err)

	if _, err := os.Stat(bdd); err == nil {
		for _, format := range []string{"md5", "sha1", "sha256", "sha512"} {
			b.Run(fmt.Sprintf("suitcase_calculate_hashes_%v", format), func(b *testing.B) {
				for _, location := range allFiles {
					fh, oerr := os.Open(location)
					require.NoError(b, oerr)
					defer func() { _ = fh.Close() }()
					_, _ = porter.CalculateHash(fh, format) // Benchmark test
				}
			})
		}
	} else {
		logger.Warn("error encountered while benchmarking", "error", err)
	}
}

func TestUint64ToInt64(t *testing.T) {
	// Test normal values
	testCases := []struct {
		name     string
		input    uint64
		expected int64
		panics   bool
	}{
		{
			name:     "zero value",
			input:    0,
			expected: 0,
			panics:   false,
		},
		{
			name:     "small positive value",
			input:    42,
			expected: 42,
			panics:   false,
		},
		{
			name:     "max int64 value",
			input:    math.MaxInt64,
			expected: math.MaxInt64,
			panics:   false,
		},
		{
			name:     "large value within range",
			input:    1000000000000,
			expected: 1000000000000,
			panics:   false,
		},
		{
			name:   "value exceeding int64 max",
			input:  math.MaxInt64 + 1,
			panics: true,
		},
		{
			name:   "max uint64 value",
			input:  math.MaxUint64,
			panics: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.panics {
				assert.Panics(t, func() {
					uint64ToInt64(tc.input)
				}, "Should panic for value out of int64 range")
			} else {
				result := uint64ToInt64(tc.input)
				assert.Equal(t, tc.expected, result)
			}
		})
	}
}

func TestUint64ToInt64EdgeCases(t *testing.T) {
	// Test boundary conditions
	
	// Just under the limit
	justUnderMax := uint64(math.MaxInt64)
	result := uint64ToInt64(justUnderMax)
	assert.Equal(t, int64(math.MaxInt64), result)
	
	// Just over the limit should panic
	assert.Panics(t, func() {
		uint64ToInt64(uint64(math.MaxInt64) + 1)
	})
	
	// Test with actual memory limit values (common use case)
	memoryLimits := []uint64{
		1024,             // 1KB
		1024 * 1024,      // 1MB  
		1024 * 1024 * 1024, // 1GB
		8 * 1024 * 1024 * 1024, // 8GB
	}
	
	for _, limit := range memoryLimits {
		result := uint64ToInt64(limit)
		assert.Equal(t, int64(limit), result)
		assert.GreaterOrEqual(t, result, int64(0))
	}
}
