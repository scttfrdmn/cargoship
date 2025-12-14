package chunking

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAdaptiveTargetCalculator(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()
	require.NotNil(t, calc)

	// Check default values
	assert.Equal(t, int64(1*1024*1024*1024), calc.smallWorkloadThreshold)   // 1GB
	assert.Equal(t, int64(10*1024*1024*1024), calc.mediumWorkloadThreshold) // 10GB
	assert.Equal(t, 10, calc.smallWorkloadChunkSizeMB)
	assert.Equal(t, 20, calc.mediumWorkloadChunkSizeMB)
	assert.Equal(t, 64, calc.largeWorkloadChunkSizeMB)
	assert.Equal(t, 8, calc.shardCount)
	assert.Equal(t, 6, calc.minChunksPerShard)
}

func TestNewAdaptiveTargetCalculatorWithConfig(t *testing.T) {
	calc := NewAdaptiveTargetCalculatorWithConfig(
		2,  // smallThresholdGB
		20, // mediumThresholdGB
		8,  // smallChunkMB
		16, // mediumChunkMB
		32, // largeChunkMB
		16, // shardCount
	)
	require.NotNil(t, calc)

	assert.Equal(t, int64(2*1024*1024*1024), calc.smallWorkloadThreshold)
	assert.Equal(t, int64(20*1024*1024*1024), calc.mediumWorkloadThreshold)
	assert.Equal(t, 8, calc.smallWorkloadChunkSizeMB)
	assert.Equal(t, 16, calc.mediumWorkloadChunkSizeMB)
	assert.Equal(t, 32, calc.largeWorkloadChunkSizeMB)
	assert.Equal(t, 16, calc.shardCount)
}

func TestCalculateOptimalChunkSize_SmallWorkload(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	// Test small workload (<1GB)
	tests := []struct {
		name     string
		sizeMB   int64
		expected int
	}{
		{"100MB", 100, 10},
		{"500MB", 500, 10},
		{"1000MB (exactly 1GB)", 1000, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeBytes := tt.sizeMB * 1024 * 1024
			chunkSize := calc.CalculateOptimalChunkSize(sizeBytes)
			assert.Equal(t, tt.expected, chunkSize, "Expected %dMB chunks for %dMB workload", tt.expected, tt.sizeMB)
		})
	}
}

func TestCalculateOptimalChunkSize_MediumWorkload(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	// Test medium workload (1GB-10GB)
	tests := []struct {
		name     string
		sizeGB   float64
		expected int
	}{
		{"1.5GB", 1.5, 20},
		{"5GB", 5.0, 20},
		{"10GB (exactly)", 10.0, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeBytes := int64(tt.sizeGB * 1024 * 1024 * 1024)
			chunkSize := calc.CalculateOptimalChunkSize(sizeBytes)
			assert.Equal(t, tt.expected, chunkSize, "Expected %dMB chunks for %.1fGB workload", tt.expected, tt.sizeGB)
		})
	}
}

func TestCalculateOptimalChunkSize_LargeWorkload(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	// Test large workload (>10GB)
	tests := []struct {
		name     string
		sizeGB   float64
		expected int
	}{
		{"15GB", 15.0, 64},
		{"50GB", 50.0, 64},
		{"100GB", 100.0, 64},
		{"1TB", 1024.0, 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeBytes := int64(tt.sizeGB * 1024 * 1024 * 1024)
			chunkSize := calc.CalculateOptimalChunkSize(sizeBytes)
			assert.Equal(t, tt.expected, chunkSize, "Expected %dMB chunks for %.1fGB workload", tt.expected, tt.sizeGB)
		})
	}
}

func TestCalculateOptimalChunkSizeWithRationale(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	tests := []struct {
		name              string
		sizeGB            float64
		expectedChunkSize int
		rationaleContains []string
	}{
		{
			name:              "SmallWorkload",
			sizeGB:            0.5,
			expectedChunkSize: 10,
			rationaleContains: []string{"Small workload", "0.50 GB", "10MB chunks", "maximum load balance"},
		},
		{
			name:              "MediumWorkload",
			sizeGB:            5.0,
			expectedChunkSize: 20,
			rationaleContains: []string{"Medium workload", "5.00 GB", "20MB chunks", "balanced cost/performance"},
		},
		{
			name:              "LargeWorkload",
			sizeGB:            50.0,
			expectedChunkSize: 64,
			rationaleContains: []string{"Large workload", "50.00 GB", "64MB chunks", "minimum API cost", "saves"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeBytes := int64(tt.sizeGB * 1024 * 1024 * 1024)
			chunkSize, rationale := calc.CalculateOptimalChunkSizeWithRationale(sizeBytes)

			assert.Equal(t, tt.expectedChunkSize, chunkSize)
			for _, expected := range tt.rationaleContains {
				assert.Contains(t, rationale, expected, "Rationale should contain '%s'", expected)
			}
		})
	}
}

func TestValidateLoadBalancing_GoodBalance(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	// Large workload with 64MB chunks should have excellent load balance
	sizeBytes := int64(50 * 1024 * 1024 * 1024) // 50GB
	chunkSizeMB := 64

	valid, message := calc.ValidateLoadBalancing(sizeBytes, chunkSizeMB)
	assert.True(t, valid, "Load balancing should be valid for large workload")
	assert.Contains(t, message, "Load balancing OK")
}

func TestValidateLoadBalancing_PoorBalance(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	// Small workload with large chunks will have poor load balance
	sizeBytes := int64(200 * 1024 * 1024) // 200MB
	chunkSizeMB := 64                     // 64MB chunks

	valid, message := calc.ValidateLoadBalancing(sizeBytes, chunkSizeMB)
	assert.False(t, valid, "Load balancing should be poor for small workload with large chunks")
	assert.Contains(t, message, "WARNING")
}

func TestValidateLoadBalancing_BoundaryConditions(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	// Minimum acceptable: 8 shards × 6 chunks/shard = 48 total chunks
	minAcceptableChunks := calc.shardCount * calc.minChunksPerShard
	sizeBytes := int64(minAcceptableChunks * 20 * 1024 * 1024) // Exactly 48 chunks @ 20MB
	chunkSizeMB := 20

	valid, message := calc.ValidateLoadBalancing(sizeBytes, chunkSizeMB)
	assert.True(t, valid, "Load balancing should be valid at minimum threshold")
	assert.Contains(t, message, "Load balancing OK")

	// Just below threshold
	sizeBytes = int64((minAcceptableChunks - 1) * 20 * 1024 * 1024) // 47 chunks @ 20MB
	valid, message = calc.ValidateLoadBalancing(sizeBytes, chunkSizeMB)
	assert.False(t, valid, "Load balancing should be invalid below threshold")
	assert.Contains(t, message, "WARNING")
}

func TestEstimateAPICost(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	tests := []struct {
		name        string
		sizeGB      float64
		chunkSizeMB int
		maxCost     float64 // Maximum expected cost
	}{
		{"1GB with 10MB chunks", 1.0, 10, 0.001},
		{"1GB with 20MB chunks", 1.0, 20, 0.0005},
		{"1GB with 64MB chunks", 1.0, 64, 0.0002},
		{"100GB with 64MB chunks", 100.0, 64, 0.010},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sizeBytes := int64(tt.sizeGB * 1024 * 1024 * 1024)
			cost := calc.EstimateAPICost(sizeBytes, tt.chunkSizeMB)

			assert.Greater(t, cost, 0.0, "Cost should be positive")
			assert.LessOrEqual(t, cost, tt.maxCost, "Cost should not exceed maximum")
		})
	}
}

func TestEstimateAPICost_CostReduction(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	sizeBytes := int64(10 * 1024 * 1024 * 1024) // 10GB

	cost10MB := calc.EstimateAPICost(sizeBytes, 10)
	cost20MB := calc.EstimateAPICost(sizeBytes, 20)
	cost64MB := calc.EstimateAPICost(sizeBytes, 64)

	// Verify cost reduction with larger chunks
	assert.Less(t, cost20MB, cost10MB, "20MB chunks should be cheaper than 10MB")
	assert.Less(t, cost64MB, cost20MB, "64MB chunks should be cheaper than 20MB")

	// Verify approximate 50% reduction for 2x chunk size
	ratio20to10 := cost20MB / cost10MB
	assert.InDelta(t, 0.5, ratio20to10, 0.01, "20MB should be ~50% cost of 10MB")
}

func TestCompareAPICosts(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	sizeBytes := int64(50 * 1024 * 1024 * 1024) // 50GB
	comparison := calc.CompareAPICosts(sizeBytes)

	// Verify output contains all expected chunk sizes
	assert.Contains(t, comparison, "10MB chunks")
	assert.Contains(t, comparison, "20MB chunks")
	assert.Contains(t, comparison, "64MB chunks")
	assert.Contains(t, comparison, "Optimal")

	// Verify percentage calculations are present
	assert.Contains(t, comparison, "% savings")

	// Verify cost amounts are present
	assert.Contains(t, comparison, "$")
}

func TestGetConfig(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()
	config := calc.GetConfig()

	// Verify all expected keys are present
	assert.Contains(t, config, "small_workload_threshold_gb")
	assert.Contains(t, config, "medium_workload_threshold_gb")
	assert.Contains(t, config, "small_workload_chunk_size_mb")
	assert.Contains(t, config, "medium_workload_chunk_size_mb")
	assert.Contains(t, config, "large_workload_chunk_size_mb")
	assert.Contains(t, config, "shard_count")
	assert.Contains(t, config, "min_chunks_per_shard")

	// Verify values match defaults
	assert.Equal(t, 1.0, config["small_workload_threshold_gb"])
	assert.Equal(t, 10.0, config["medium_workload_threshold_gb"])
	assert.Equal(t, 10, config["small_workload_chunk_size_mb"])
	assert.Equal(t, 20, config["medium_workload_chunk_size_mb"])
	assert.Equal(t, 64, config["large_workload_chunk_size_mb"])
	assert.Equal(t, 8, config["shard_count"])
	assert.Equal(t, 6, config["min_chunks_per_shard"])
}

func TestAdaptiveTargetCalculator_RealWorldScenarios(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	scenarios := []struct {
		name              string
		description       string
		sizeGB            float64
		expectedChunkSize int
		shouldBalance     bool
	}{
		{
			name:              "Phase3.2Workload",
			description:       "Actual Phase 3.2 benchmark (50K files @ 1GB)",
			sizeGB:            0.95, // 976.56 MB compressed
			expectedChunkSize: 10,
			shouldBalance:     true,
		},
		{
			name:              "SmallFilesWorkload",
			description:       "10K small files @ 185MB compressed",
			sizeGB:            0.18,
			expectedChunkSize: 10,
			shouldBalance:     false, // Only ~18 chunks @ 10MB
		},
		{
			name:              "LargeFilesWorkload",
			description:       "100 large files @ 56GB compressed",
			sizeGB:            56.0,
			expectedChunkSize: 64,
			shouldBalance:     true,
		},
		{
			name:              "EnterpriseWorkload",
			description:       "1TB compressed data",
			sizeGB:            1024.0,
			expectedChunkSize: 64,
			shouldBalance:     true,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			sizeBytes := int64(scenario.sizeGB * 1024 * 1024 * 1024)

			// Test chunk size calculation
			chunkSize := calc.CalculateOptimalChunkSize(sizeBytes)
			assert.Equal(t, scenario.expectedChunkSize, chunkSize,
				"%s: Expected %dMB chunks", scenario.description, scenario.expectedChunkSize)

			// Test rationale generation
			_, rationale := calc.CalculateOptimalChunkSizeWithRationale(sizeBytes)
			assert.NotEmpty(t, rationale, "%s: Rationale should not be empty", scenario.description)

			// Test load balancing validation
			valid, message := calc.ValidateLoadBalancing(sizeBytes, chunkSize)
			if scenario.shouldBalance {
				assert.True(t, valid, "%s: Load balancing should be good. Message: %s", scenario.description, message)
			} else {
				// Small workloads may have poor balance
				t.Logf("%s: Load balance warning (expected for small workloads): %s", scenario.description, message)
			}

			// Test API cost estimation
			cost := calc.EstimateAPICost(sizeBytes, chunkSize)
			assert.Greater(t, cost, 0.0, "%s: API cost should be positive", scenario.description)

			// Log comparison for visibility
			comparison := calc.CompareAPICosts(sizeBytes)
			t.Logf("\n%s:\n%s", scenario.description, comparison)
		})
	}
}

func TestAdaptiveTargetCalculator_CostSavingsValidation(t *testing.T) {
	calc := NewAdaptiveTargetCalculator()

	// Enterprise workload: 1 PB/month
	sizeBytes := int64(1000 * 1024 * 1024 * 1024 * 1024) // 1 PB

	cost20MB := calc.EstimateAPICost(sizeBytes, 20)
	cost64MB := calc.EstimateAPICost(sizeBytes, 64)

	savings := cost20MB - cost64MB
	savingsPercent := (savings / cost20MB) * 100

	t.Logf("Enterprise workload (1 PB):")
	t.Logf("  20MB chunks: $%.2f", cost20MB)
	t.Logf("  64MB chunks: $%.2f", cost64MB)
	t.Logf("  Savings: $%.2f (%.1f%%)", savings, savingsPercent)

	// Verify savings match expected from cost analysis document
	assert.InDelta(t, 69.0, savingsPercent, 1.0, "Should save ~69%% with 64MB chunks")
	assert.Greater(t, savings, 150.0, "Should save >$150/month for 1PB workload")
}

// Benchmark tests
func BenchmarkCalculateOptimalChunkSize(b *testing.B) {
	calc := NewAdaptiveTargetCalculator()
	sizeBytes := int64(50 * 1024 * 1024 * 1024) // 50GB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calc.CalculateOptimalChunkSize(sizeBytes)
	}
}

func BenchmarkCalculateOptimalChunkSizeWithRationale(b *testing.B) {
	calc := NewAdaptiveTargetCalculator()
	sizeBytes := int64(50 * 1024 * 1024 * 1024) // 50GB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = calc.CalculateOptimalChunkSizeWithRationale(sizeBytes)
	}
}

func BenchmarkValidateLoadBalancing(b *testing.B) {
	calc := NewAdaptiveTargetCalculator()
	sizeBytes := int64(50 * 1024 * 1024 * 1024) // 50GB
	chunkSizeMB := 64

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = calc.ValidateLoadBalancing(sizeBytes, chunkSizeMB)
	}
}

func BenchmarkCompareAPICosts(b *testing.B) {
	calc := NewAdaptiveTargetCalculator()
	sizeBytes := int64(50 * 1024 * 1024 * 1024) // 50GB

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = calc.CompareAPICosts(sizeBytes)
	}
}

// Helper function to format size for logging
var _ = formatSize // Prevent unused warning

func formatSize(sizeBytes int64) string {
	if sizeBytes < 1024*1024 {
		return strings.TrimSpace(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f KB", float64(sizeBytes)/1024), "0"), "."))
	} else if sizeBytes < 1024*1024*1024 {
		return strings.TrimSpace(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f MB", float64(sizeBytes)/(1024*1024)), "0"), "."))
	} else {
		return strings.TrimSpace(strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f GB", float64(sizeBytes)/(1024*1024*1024)), "0"), "."))
	}
}
