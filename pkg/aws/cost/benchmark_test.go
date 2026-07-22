package cost

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBenchmarkCostCalculator(t *testing.T) {
	pricingCfg := &config.PricingConfig{
		Currency:             "USD",
		PricingCacheDuration: "1h",
	}

	pm, err := NewPricingManager(pricingCfg, aws.Config{}, nil)
	require.NoError(t, err)

	bcc := NewBenchmarkCostCalculator(pm, "us-west-2")
	require.NotNil(t, bcc)
	assert.Equal(t, "us-west-2", bcc.region)
	assert.NotNil(t, bcc.pricingManager)
}

func TestCalculateCompetitorCost(t *testing.T) {
	pricingCfg := &config.PricingConfig{
		Currency:             "USD",
		PricingCacheDuration: "1h",
	}

	pm, err := NewPricingManager(pricingCfg, aws.Config{}, nil)
	require.NoError(t, err)

	bcc := NewBenchmarkCostCalculator(pm, "us-west-2")

	ctx := context.Background()
	comparison, err := bcc.CalculateCompetitorCost(ctx, "test-scenario", "s5cmd", 100.0, 10000)
	require.NoError(t, err)
	require.NotNil(t, comparison)

	assert.Equal(t, "test-scenario", comparison.Scenario)
	assert.Equal(t, "s5cmd", comparison.Tool)
	assert.Equal(t, "USD", comparison.Currency)

	// Data transfer should be free (0 for uploads)
	assert.Equal(t, 0.0, comparison.DataTransferCost)

	// PUT requests: 10000 files = 10000 requests
	// Price: $0.005 per 1000 requests * 10 = $0.05 total
	assert.InDelta(t, 0.05, comparison.PUTRequestCost, 0.001)

	// Storage: 100 GB * $0.023 per GB-month = $2.30/month
	assert.InDelta(t, 2.30, comparison.StorageCostMonthly, 0.10)

	// Upload cost = transfer + requests
	assert.InDelta(t, 0.05, comparison.TotalUploadCost, 0.001)

	// Annual TCO = upload + (12 * monthly storage)
	expectedAnnualTCO := 0.05 + (2.30 * 12)
	assert.InDelta(t, expectedAnnualTCO, comparison.AnnualTCO, 0.50)
}

func TestCalculateCargoShipCost(t *testing.T) {
	pricingCfg := &config.PricingConfig{
		Currency:             "USD",
		PricingCacheDuration: "1h",
	}

	pm, err := NewPricingManager(pricingCfg, aws.Config{}, nil)
	require.NoError(t, err)

	bcc := NewBenchmarkCostCalculator(pm, "us-west-2")

	ctx := context.Background()

	tests := []struct {
		name                string
		sizeGB              float64
		fileCount           int64
		compressionRatio    float64
		deduplicationRatio  float64
		storageClass        config.StorageClass
		wantEffectiveSizeGB float64
	}{
		{
			name:                "no compression or dedup",
			sizeGB:              100.0,
			fileCount:           10000,
			compressionRatio:    1.0,
			deduplicationRatio:  1.0,
			storageClass:        config.StorageClassStandard,
			wantEffectiveSizeGB: 100.0,
		},
		{
			name:                "with compression",
			sizeGB:              100.0,
			fileCount:           10000,
			compressionRatio:    3.0,
			deduplicationRatio:  1.0,
			storageClass:        config.StorageClassStandard,
			wantEffectiveSizeGB: 33.33,
		},
		{
			name:                "with compression and dedup",
			sizeGB:              100.0,
			fileCount:           10000,
			compressionRatio:    2.0,
			deduplicationRatio:  2.0,
			storageClass:        config.StorageClassStandard,
			wantEffectiveSizeGB: 25.0,
		},
		{
			name:                "with glacier tier",
			sizeGB:              100.0,
			fileCount:           10000,
			compressionRatio:    1.0,
			deduplicationRatio:  1.0,
			storageClass:        config.StorageClassGlacier,
			wantEffectiveSizeGB: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comparison, err := bcc.CalculateCargoShipCost(
				ctx,
				tt.name,
				tt.sizeGB,
				tt.fileCount,
				tt.compressionRatio,
				tt.deduplicationRatio,
				tt.storageClass,
			)
			require.NoError(t, err)
			require.NotNil(t, comparison)

			assert.Equal(t, tt.name, comparison.Scenario)
			assert.Equal(t, "cargoship", comparison.Tool)
			assert.NotNil(t, comparison.CargoShipAdvantages)

			// Verify compression ratio is recorded
			assert.Equal(t, tt.compressionRatio, comparison.CargoShipAdvantages.CompressionRatio)
			assert.Equal(t, tt.deduplicationRatio, comparison.CargoShipAdvantages.DeduplicationRatio)

			// Verify cost is less than competitor when optimizations are enabled
			if tt.compressionRatio > 1.0 || tt.deduplicationRatio > 1.0 {
				// Should have savings
				assert.Greater(t, comparison.CargoShipAdvantages.TotalSavings, 0.0)
			}

			// Storage class should be recorded
			assert.Equal(t, string(tt.storageClass), comparison.CargoShipAdvantages.StorageTierUsed)
		})
	}
}

func TestCalculateAdvantages(t *testing.T) {
	pricingCfg := &config.PricingConfig{
		Currency:             "USD",
		PricingCacheDuration: "1h",
	}

	pm, err := NewPricingManager(pricingCfg, aws.Config{}, nil)
	require.NoError(t, err)

	bcc := NewBenchmarkCostCalculator(pm, "us-west-2")
	ctx := context.Background()

	advantages := bcc.calculateAdvantages(
		ctx,
		100.0,                      // original size
		33.33,                      // effective size (3:1 compression)
		10000,                      // original file count
		334,                        // actual chunks (100GB / 0.3 = ~334 chunks of 100MB)
		3.0,                        // compression ratio
		1.0,                        // no dedup
		config.StorageClassGlacier, // cheaper tier
	)

	require.NotNil(t, advantages)

	// Should have compression savings
	assert.Greater(t, advantages.CompressionSavings, 0.0)

	// Should have chunking savings (fewer requests)
	assert.Greater(t, advantages.ChunkingSavings, 0.0)
	assert.Equal(t, int64(10000-334), advantages.RequestReduction)

	// Should have storage tier savings (Glacier vs Standard)
	assert.Greater(t, advantages.StorageTierSavings, 0.0)
	assert.Equal(t, "GLACIER", advantages.StorageTierUsed)

	// No dedup savings in this test
	assert.Equal(t, 0.0, advantages.DeduplicationSavings)

	// Total savings should be sum of all
	expectedTotal := advantages.CompressionSavings +
		advantages.ChunkingSavings +
		advantages.StorageTierSavings +
		advantages.DeduplicationSavings
	assert.InDelta(t, expectedTotal, advantages.TotalSavings, 0.01)

	// Should have positive savings percentage
	assert.Greater(t, advantages.SavingsPercentage, 0.0)
}

func TestCompareCosts(t *testing.T) {
	pricingCfg := &config.PricingConfig{
		Currency:             "USD",
		PricingCacheDuration: "1h",
	}

	pm, err := NewPricingManager(pricingCfg, aws.Config{}, nil)
	require.NoError(t, err)

	bcc := NewBenchmarkCostCalculator(pm, "us-west-2")
	ctx := context.Background()

	// Calculate competitor cost
	competitorCost, err := bcc.CalculateCompetitorCost(ctx, "test-scenario", "s5cmd", 100.0, 10000)
	require.NoError(t, err)

	// Calculate CargoShip cost with optimizations
	cargoshipCost, err := bcc.CalculateCargoShipCost(
		ctx,
		"test-scenario",
		100.0,
		10000,
		3.0,                        // 3:1 compression
		1.0,                        // no dedup
		config.StorageClassGlacier, // cheaper tier
	)
	require.NoError(t, err)

	// Compare costs
	comparison := bcc.CompareCosts(cargoshipCost, competitorCost)
	require.NotNil(t, comparison)

	assert.Equal(t, "test-scenario", comparison["scenario"])

	// CargoShip should have lower costs
	assert.Less(t, comparison["cargoship_upload_cost"], comparison["competitor_upload_cost"])
	assert.Less(t, comparison["cargoship_monthly_cost"], comparison["competitor_monthly_cost"])
	assert.Less(t, comparison["cargoship_annual_tco"], comparison["competitor_annual_tco"])

	// Savings should be positive
	assert.Greater(t, comparison["upload_cost_savings"], 0.0)
	assert.Greater(t, comparison["monthly_cost_savings"], 0.0)
	assert.Greater(t, comparison["annual_tco_savings"], 0.0)

	// Verify advantages are included
	assert.NotNil(t, comparison["cargoship_advantages"])
}

func TestGenerateCostReport(t *testing.T) {
	pricingCfg := &config.PricingConfig{
		Currency:             "USD",
		PricingCacheDuration: "1h",
	}

	pm, err := NewPricingManager(pricingCfg, aws.Config{}, nil)
	require.NoError(t, err)

	bcc := NewBenchmarkCostCalculator(pm, "us-west-2")

	comparisons := []*BenchmarkCostComparison{
		{
			Scenario:           "test-scenario-1",
			Tool:               "cargoship",
			TotalUploadCost:    0.10,
			MonthlyRunningCost: 0.50,
			AnnualTCO:          6.10,
			Currency:           "USD",
			CargoShipAdvantages: &CargoShipCostAdvantage{
				CompressionSavings: 15.00,
				CompressionRatio:   3.0,
				ChunkingSavings:    0.50,
				RequestReduction:   9000,
				StorageTierSavings: 10.00,
				StorageTierUsed:    "GLACIER_IR",
				TotalSavings:       25.50,
				SavingsPercentage:  40.0,
			},
		},
	}

	report := bcc.GenerateCostReport(comparisons)
	assert.NotEmpty(t, report)

	// Verify report contains expected sections
	assert.Contains(t, report, "# Cost Analysis Report")
	assert.Contains(t, report, "Region: us-west-2")
	assert.Contains(t, report, "Currency: USD")
	assert.Contains(t, report, "test-scenario-1")
	assert.Contains(t, report, "CargoShip Advantages:")
	assert.Contains(t, report, "Compression")
	assert.Contains(t, report, "Intelligent Chunking")
	assert.Contains(t, report, "Storage Tier")
	assert.Contains(t, report, "Total Savings")
}
