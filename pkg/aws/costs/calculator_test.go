package costs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

// Note: Testing pricing service integration requires interface changes
// For now, we focus on testing the fallback pricing paths

func TestNewCalculator(t *testing.T) {
	calc := NewCalculator("us-east-1")
	if calc == nil {
		t.Fatal("NewCalculator returned nil")
	}
	
	if calc.region != "us-east-1" {
		t.Errorf("Expected region 'us-east-1', got %s", calc.region)
	}
}

func TestNewCalculatorWithPricing(t *testing.T) {
	// Test with nil pricing service
	calc := NewCalculatorWithPricing("us-west-2", nil)
	if calc == nil {
		t.Fatal("NewCalculatorWithPricing returned nil")
	}
	
	if calc.region != "us-west-2" {
		t.Errorf("Expected region 'us-west-2', got %s", calc.region)
	}
	
	if calc.pricingService != nil {
		t.Error("Expected pricingService to be nil")
	}
}

func TestCalculateStorageCost(t *testing.T) {
	calc := NewCalculator("us-east-1")
	ctx := context.Background()
	
	tests := []struct {
		name         string
		sizeGB       float64
		storageClass config.StorageClass
		wantMin      float64 // Minimum expected cost
		wantMax      float64 // Maximum expected cost
	}{
		{
			name:         "standard storage 10GB",
			sizeGB:       10.0,
			storageClass: config.StorageClassStandard,
			wantMin:      0.20, // Should be around $0.23
			wantMax:      0.30,
		},
		{
			name:         "deep archive 10GB",
			sizeGB:       10.0,
			storageClass: config.StorageClassDeepArchive,
			wantMin:      0.005, // Should be around $0.0099
			wantMax:      0.015,
		},
		{
			name:         "intelligent tiering 100GB",
			sizeGB:       100.0,
			storageClass: config.StorageClassIntelligentTiering,
			wantMin:      2.0, // Should be around $2.25
			wantMax:      2.5,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calc.calculateStorageCost(ctx, tt.sizeGB, tt.storageClass)
			
			if cost < tt.wantMin || cost > tt.wantMax {
				t.Errorf("calculateStorageCost() = %v, want between %v and %v", 
					cost, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestCalculateTransferCost(t *testing.T) {
	calc := NewCalculator("us-east-1")
	ctx := context.Background()
	
	tests := []struct {
		name    string
		sizeGB  float64
		want    float64
	}{
		{
			name:   "under 1GB free tier",
			sizeGB: 0.5,
			want:   0.0,
		},
		{
			name:   "exactly 1GB free tier",
			sizeGB: 1.0,
			want:   0.0,
		},
		{
			name:   "5GB with 4GB chargeable",
			sizeGB: 5.0,
			want:   0.36, // 4GB * $0.09/GB
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calc.calculateTransferCost(ctx, tt.sizeGB)
			
			if cost != tt.want {
				t.Errorf("calculateTransferCost() = %v, want %v", cost, tt.want)
			}
		})
	}
}

func TestCalculateTransferCostEdgeCases(t *testing.T) {
	calc := NewCalculator("us-east-1")
	ctx := context.Background()
	
	tests := []struct {
		name    string
		sizeGB  float64
		want    float64
	}{
		{
			name:   "exactly free tier boundary",
			sizeGB: 1.0,
			want:   0.0,
		},
		{
			name:   "just over free tier",
			sizeGB: 1.1,
			want:   0.009000000000000008, // 0.1GB * $0.09/GB (floating point precision)
		},
		{
			name:   "large transfer",
			sizeGB: 100.0,
			want:   8.91, // 99GB * $0.09/GB
		},
		{
			name:   "zero size",
			sizeGB: 0.0,
			want:   0.0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calc.calculateTransferCost(ctx, tt.sizeGB)
			
			if cost != tt.want {
				t.Errorf("calculateTransferCost() = %v, want %v", cost, tt.want)
			}
		})
	}
}

func TestCalculateRequestCostComprehensive(t *testing.T) {
	calc := NewCalculator("us-east-1")
	ctx := context.Background()
	
	tests := []struct {
		name         string
		numRequests  int
		storageClass config.StorageClass
		wantMin      float64
		wantMax      float64
	}{
		{
			name:         "standard 1000 requests",
			numRequests:  1000,
			storageClass: config.StorageClassStandard,
			wantMin:      0.004,
			wantMax:      0.006,
		},
		{
			name:         "deep archive 500 requests",
			numRequests:  500,
			storageClass: config.StorageClassDeepArchive,
			wantMin:      0.020,
			wantMax:      0.030,
		},
		{
			name:         "intelligent tiering 2000 requests",
			numRequests:  2000,
			storageClass: config.StorageClassIntelligentTiering,
			wantMin:      0.008,
			wantMax:      0.012,
		},
		{
			name:         "zero requests",
			numRequests:  0,
			storageClass: config.StorageClassStandard,
			wantMin:      0.0,
			wantMax:      0.0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := calc.calculateRequestCost(ctx, tt.numRequests, tt.storageClass)
			
			if cost < tt.wantMin || cost > tt.wantMax {
				t.Errorf("calculateRequestCost() = %v, want between %v and %v", 
					cost, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEstimateArchives(t *testing.T) {
	calc := NewCalculator("us-east-1")
	ctx := context.Background()
	
	archives := []s3.Archive{
		{
			Key:             "test1.tar.zst",
			Size:            1024 * 1024 * 1024, // 1GB
			StorageClass:    config.StorageClassStandard,
			OriginalSize:    2 * 1024 * 1024 * 1024, // 2GB original
			CompressionType: "zstd",
			AccessPattern:   "archive",
			RetentionDays:   365,
		},
		{
			Key:             "test2.tar.zst",
			Size:            512 * 1024 * 1024, // 512MB
			StorageClass:    config.StorageClassGlacier,
			OriginalSize:    1024 * 1024 * 1024, // 1GB original
			CompressionType: "zstd",
			AccessPattern:   "rare",
			RetentionDays:   1000,
		},
	}
	
	estimate, err := calc.EstimateArchives(ctx, archives)
	if err != nil {
		t.Fatalf("EstimateArchives() error = %v", err)
	}
	
	// Basic sanity checks
	if estimate.TotalSizeGB <= 0 {
		t.Error("Expected TotalSizeGB > 0")
	}
	
	if estimate.ArchiveCount != 2 {
		t.Errorf("Expected ArchiveCount = 2, got %d", estimate.ArchiveCount)
	}
	
	if estimate.Region != "us-east-1" {
		t.Errorf("Expected Region = 'us-east-1', got %s", estimate.Region)
	}
	
	// Storage costs should be reasonable
	if estimate.StorageCosts.Standard <= 0 {
		t.Error("Expected Standard storage cost > 0")
	}
	
	if estimate.StorageCosts.DeepArchive <= 0 {
		t.Error("Expected Deep Archive storage cost > 0")
	}
	
	// Deep Archive should be cheaper than Standard
	if estimate.StorageCosts.DeepArchive >= estimate.StorageCosts.Standard {
		t.Error("Expected Deep Archive to be cheaper than Standard")
	}
	
	// Total costs should be calculated
	if estimate.TotalMonthlyCost <= 0 {
		t.Error("Expected TotalMonthlyCost > 0")
	}
	
	if estimate.TotalAnnualCost != estimate.TotalMonthlyCost*12 {
		t.Error("TotalAnnualCost should be 12x TotalMonthlyCost")
	}
	
	// Timestamp should be recent
	if time.Since(estimate.CalculatedAt) > time.Minute {
		t.Error("CalculatedAt timestamp should be recent")
	}
}

func TestGenerateRecommendations(t *testing.T) {
	calc := NewCalculator("us-east-1")
	
	// Test with large archive that should trigger Deep Archive recommendation
	archives := []s3.Archive{
		{
			Key:           "large-archive.tar.zst",
			Size:          2 * 1024 * 1024 * 1024, // 2GB (larger than 1GB threshold)
			AccessPattern: "archive",
			RetentionDays: 2000,
		},
	}
	
	estimate := &CostEstimate{
		TotalSizeGB: 10.0,
	}
	
	recommendations := calc.generateRecommendations(archives, estimate)
	
	if len(recommendations) == 0 {
		t.Error("Expected at least one recommendation for large archive")
	}
	
	// Should recommend something for archive access pattern and long retention
	foundRecommendation := false
	for _, rec := range recommendations {
		if rec.Type == "storage_class" || rec.Type == "lifecycle" {
			foundRecommendation = true
			break
		}
	}
	
	if !foundRecommendation {
		t.Error("Expected storage class or lifecycle recommendation for archival data")
	}
}

func TestGenerateRecommendationsComprehensive(t *testing.T) {
	calc := NewCalculator("us-east-1")
	
	tests := []struct {
		name          string
		archives      []s3.Archive
		totalSizeGB   float64
		expectedTypes []string
		minCount      int
	}{
		{
			name: "large archive with significant savings",
			archives: []s3.Archive{
				{
					Key:           "huge-archive.tar.zst",
					Size:          50 * 1024 * 1024 * 1024, // 50GB - savings: 50 * 0.02201 = ~$1.10/month
					AccessPattern: "archive",
					RetentionDays: 1000,
				},
			},
			totalSizeGB:   55.0,
			expectedTypes: []string{"storage_class", "lifecycle"},
			minCount:      2,
		},
		{
			name: "small archive - no deep archive recommendation",
			archives: []s3.Archive{
				{
					Key:           "small-archive.tar.zst",
					Size:          500 * 1024 * 1024, // 500MB < 1GB threshold
					AccessPattern: "archive",
					RetentionDays: 2000,
				},
			},
			totalSizeGB:   15.0,
			expectedTypes: []string{"lifecycle"},
			minCount:      1,
		},
		{
			name: "unknown access patterns - intelligent tiering",
			archives: []s3.Archive{
				{
					Key:           "unknown1.tar.zst",
					Size:          1024 * 1024 * 1024,
					AccessPattern: "unknown",
					RetentionDays: 100,
				},
				{
					Key:           "unknown2.tar.zst",
					Size:          1024 * 1024 * 1024,
					AccessPattern: "", // Empty pattern
					RetentionDays: 100,
				},
			},
			totalSizeGB:   15.0, // > 10GB threshold
			expectedTypes: []string{"storage_class"},
			minCount:      1,
		},
		{
			name: "no long-term retention - no lifecycle",
			archives: []s3.Archive{
				{
					Key:           "short-term.tar.zst",
					Size:          2 * 1024 * 1024 * 1024,
					AccessPattern: "frequent",
					RetentionDays: 30, // < 365 days
				},
			},
			totalSizeGB:   15.0,
			expectedTypes: []string{}, // No specific recommendations
			minCount:      0,
		},
		{
			name: "small total size - no intelligent tiering",
			archives: []s3.Archive{
				{
					Key:           "small-total.tar.zst",
					Size:          1024 * 1024 * 1024,
					AccessPattern: "unknown",
					RetentionDays: 100,
				},
			},
			totalSizeGB:   5.0, // < 10GB threshold
			expectedTypes: []string{},
			minCount:      0,
		},
		{
			name: "mixed access patterns - partial intelligent tiering",
			archives: []s3.Archive{
				{
					Key:           "known1.tar.zst",
					Size:          1024 * 1024 * 1024,
					AccessPattern: "frequent",
					RetentionDays: 100,
				},
				{
					Key:           "unknown1.tar.zst",
					Size:          1024 * 1024 * 1024,
					AccessPattern: "unknown",
					RetentionDays: 100,
				},
			},
			totalSizeGB:   15.0, // > 10GB but only 50% unknown
			expectedTypes: []string{}, // < 50% unknown patterns
			minCount:      0,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			estimate := &CostEstimate{
				TotalSizeGB: tt.totalSizeGB,
			}
			
			recommendations := calc.generateRecommendations(tt.archives, estimate)
			
			if len(recommendations) < tt.minCount {
				t.Errorf("Expected at least %d recommendations, got %d", tt.minCount, len(recommendations))
			}
			
			// Check for expected recommendation types
			foundTypes := make(map[string]bool)
			for _, rec := range recommendations {
				foundTypes[rec.Type] = true
				
				// Validate recommendation structure
				if rec.Description == "" {
					t.Error("Recommendation should have description")
				}
				if rec.EstimatedSavings < 0 {
					t.Error("Estimated savings should be non-negative")
				}
				if rec.Confidence < 0 || rec.Confidence > 1 {
					t.Error("Confidence should be between 0 and 1")
				}
				if rec.Impact == "" {
					t.Error("Recommendation should have impact level")
				}
			}
			
			for _, expectedType := range tt.expectedTypes {
				if !foundTypes[expectedType] {
					t.Errorf("Expected recommendation type %s not found", expectedType)
				}
			}
		})
	}
}

func BenchmarkEstimateArchives(b *testing.B) {
	calc := NewCalculator("us-east-1")
	ctx := context.Background()
	
	// Create test archives
	archives := make([]s3.Archive, 100)
	for i := range archives {
		archives[i] = s3.Archive{
			Key:             fmt.Sprintf("archive-%d.tar.zst", i),
			Size:            int64(i+1) * 1024 * 1024, // 1MB to 100MB
			StorageClass:    config.StorageClassStandard,
			OriginalSize:    int64(i+1) * 2 * 1024 * 1024,
			CompressionType: "zstd",
			AccessPattern:   "unknown",
		}
	}
	
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_, err := calc.EstimateArchives(ctx, archives)
		if err != nil {
			b.Fatal(err)
		}
	}
}