package costs

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/scttfrdmn/cargoship-cli/pkg/aws/config"
	"github.com/scttfrdmn/cargoship-cli/pkg/aws/s3"
)

func TestNewCalculator(t *testing.T) {
	calc := NewCalculator("us-east-1")
	if calc == nil {
		t.Fatal("NewCalculator returned nil")
	}
	
	if calc.region != "us-east-1" {
		t.Errorf("Expected region 'us-east-1', got %s", calc.region)
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