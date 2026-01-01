package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/pricing"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/costs"
	pricingpkg "github.com/scttfrdmn/cargoship/pkg/aws/pricing"
	"github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

var (
	estimateFormat           string
	estimateStorageClass     string
	showRecommendations      bool
	estimateRegion           string
	useRealTimePricing       bool
	showParallelOptimization bool
	estimateMaxPrefixes      int
	showUploadOptimization   bool
	estimateBandwidth        float64
	showComparison           bool // Issue #169: Show chunking benefits comparison
)

// NewEstimateCmd creates the estimate command for cost calculation
func NewEstimateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "estimate [path]",
		Short: "Estimate AWS costs for archiving data",
		Long: `Estimate the cost of archiving data to AWS S3.

This command analyzes the specified directory and provides detailed cost estimates
for different storage classes, including storage, transfer, and request costs.

Examples:
  cargoship estimate ./research-data
  cargoship estimate /data --storage-class glacier --format json
  cargoship estimate . --show-recommendations --region us-west-2`,
		Args: cobra.ExactArgs(1),
		RunE: runEstimate,
	}

	cmd.Flags().StringVarP(&estimateFormat, "format", "f", "table", "Output format (table, json)")
	cmd.Flags().StringVarP(&estimateStorageClass, "storage-class", "s", "", "Target storage class for estimation")
	cmd.Flags().BoolVar(&showRecommendations, "show-recommendations", true, "Show cost optimization recommendations")
	cmd.Flags().StringVar(&estimateRegion, "region", "us-east-1", "AWS region for cost calculation")
	cmd.Flags().BoolVar(&useRealTimePricing, "real-time-pricing", false, "Use real-time AWS pricing (requires AWS credentials)")
	cmd.Flags().BoolVar(&showParallelOptimization, "show-parallel", true, "Show parallel upload optimization recommendations")
	cmd.Flags().IntVar(&estimateMaxPrefixes, "max-prefixes", 0, "Maximum prefixes for parallel upload analysis (0 = auto)")
	cmd.Flags().BoolVar(&showUploadOptimization, "show-upload-optimization", true, "Show intelligent upload sizing recommendations")
	cmd.Flags().Float64Var(&estimateBandwidth, "bandwidth", 0, "Network bandwidth in MB/s for optimization (0 = auto-detect)")
	cmd.Flags().BoolVar(&showComparison, "show-comparison", false, "Show cost comparison: naive upload vs CargoShip chunking (Issue #169)")

	return cmd
}

func runEstimate(cmd *cobra.Command, args []string) error {
	sourcePath := args[0]

	// Validate path exists
	if _, err := os.Stat(sourcePath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", sourcePath)
	}

	// Create a mock inventory for cost estimation
	archives, fileCount, err := createMockArchives(sourcePath)
	if err != nil {
		return fmt.Errorf("failed to analyze directory: %w", err)
	}

	if len(archives) == 0 {
		fmt.Println("No files found to archive")
		return nil
	}

	// Create cost calculator with optional real-time pricing
	var calculator *costs.Calculator
	if useRealTimePricing {
		calculator = createCalculatorWithRealTimePricing(context.Background(), estimateRegion)
	} else {
		calculator = costs.NewCalculator(estimateRegion)
	}

	// Calculate costs
	ctx := context.Background()

	// Issue #169: Show comparison if requested
	var comparison *costs.ComparisonEstimate
	if showComparison {
		// Determine storage class for comparison
		storageClass := config.StorageClassIntelligentTiering
		if estimateStorageClass != "" {
			storageClass = config.StorageClass(strings.ToUpper(estimateStorageClass))
		}

		// Calculate comparison
		totalSizeGB := float64(archives[0].Size) / (1024 * 1024 * 1024)
		comparison, err = calculator.EstimateWithComparison(ctx, fileCount, totalSizeGB, storageClass)
		if err != nil {
			return fmt.Errorf("failed to calculate comparison: %w", err)
		}
	}

	estimate, err := calculator.EstimateArchives(ctx, archives)
	if err != nil {
		return fmt.Errorf("failed to calculate costs: %w", err)
	}

	// Generate parallel upload optimization if requested
	var parallelOpt *s3.PrefixOptimization
	if showParallelOptimization {
		parallelOpt = generateParallelOptimization(archives)
	}

	// Generate upload optimization recommendations if requested
	var uploadOpt *s3.UploadRecommendations
	if showUploadOptimization {
		uploadOpt = generateUploadOptimization(archives)
	}

	// Output results
	switch estimateFormat {
	case "json":
		return outputJSON(estimate, parallelOpt, uploadOpt, comparison)
	case "table":
		return outputTable(estimate, parallelOpt, uploadOpt, comparison, sourcePath)
	default:
		return fmt.Errorf("unsupported format: %s", estimateFormat)
	}
}

// createMockArchives creates mock archives for cost estimation based on directory analysis
func createMockArchives(sourcePath string) ([]s3.Archive, int, error) {
	var archives []s3.Archive
	var totalSize int64
	var fileCount int

	// Walk directory to calculate total size and file count
	err := filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
			fileCount++
		}
		return nil
	})

	if err != nil {
		return nil, 0, err
	}

	// Create mock archive (simplified - in reality we'd use the inventory system)
	archive := s3.Archive{
		Key:             filepath.Base(sourcePath) + ".tar.zst",
		Size:            totalSize,
		StorageClass:    config.StorageClassIntelligentTiering,
		OriginalSize:    totalSize,
		CompressionType: "zstd",
		AccessPattern:   "unknown", // Default for estimation
		RetentionDays:   365,       // Default 1 year retention
	}

	// Override storage class if specified
	if estimateStorageClass != "" {
		archive.StorageClass = config.StorageClass(strings.ToUpper(estimateStorageClass))
	}

	archives = append(archives, archive)
	return archives, fileCount, nil
}

// createCalculatorWithRealTimePricing creates a calculator with AWS Pricing API integration
func createCalculatorWithRealTimePricing(ctx context.Context, region string) *costs.Calculator {
	// Load AWS config for pricing API (requires credentials)
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion("us-east-1")) // Pricing API only in us-east-1
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to load AWS config for real-time pricing: %v\n", err)
		fmt.Fprintf(os.Stderr, "Using fallback pricing...\n")
		return costs.NewCalculator(region)
	}

	// Create pricing service
	pricingClient := pricing.NewFromConfig(cfg)
	pricingService := pricingpkg.NewService(pricingClient)

	return costs.NewCalculatorWithPricing(region, pricingService)
}

// generateParallelOptimization creates parallel upload recommendations
func generateParallelOptimization(archives []s3.Archive) *s3.PrefixOptimization {
	// Create a temporary parallel uploader for optimization analysis
	parallelConfig := s3.ParallelConfig{
		MaxPrefixes:        estimateMaxPrefixes,
		PrefixPattern:      "hash", // Default for analysis
		LoadBalancing:      "least_loaded",
		PrefixOptimization: true,
	}

	// Create minimal transporter for analysis (not used for actual upload)
	uploader := s3.NewParallelUploader(nil, parallelConfig)

	return uploader.OptimizePrefixDistribution(archives)
}

// generateUploadOptimization creates intelligent upload sizing recommendations
func generateUploadOptimization(archives []s3.Archive) *s3.UploadRecommendations {
	if len(archives) == 0 {
		return nil
	}

	// Use first archive for analysis (in real usage, would analyze all)
	archive := archives[0]

	// Create adaptive uploader for analysis
	adaptiveConfig := s3.AdaptiveConfig{
		MinChunkSize:                  5 * 1024 * 1024,   // 5MB
		MaxChunkSize:                  100 * 1024 * 1024, // 100MB
		MaxConcurrency:                10,
		EnableContentTypeOptimization: true,
	}

	adaptiveUploader := s3.NewAdaptiveUploader(nil, adaptiveConfig)

	// Simulate network conditions if bandwidth provided
	if estimateBandwidth > 0 {
		sample := s3.NetworkSample{
			Timestamp: time.Now(),
			Bandwidth: estimateBandwidth,
			Latency:   100 * time.Millisecond, // Default assumption
			Success:   true,
		}
		adaptiveUploader.RecordNetworkSample(sample)
	}

	// Get recommendations for the archive
	return adaptiveUploader.GetRecommendations(archive.Size, "application/octet-stream")
}

func outputJSON(estimate *costs.CostEstimate, parallelOpt *s3.PrefixOptimization, uploadOpt *s3.UploadRecommendations, comparison *costs.ComparisonEstimate) error {
	output := map[string]interface{}{
		"cost_estimate": estimate,
	}

	if parallelOpt != nil {
		output["parallel_optimization"] = parallelOpt
	}

	if uploadOpt != nil {
		output["upload_optimization"] = uploadOpt
	}

	// Issue #169: Include comparison if available
	if comparison != nil {
		output["cost_comparison"] = comparison
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

func outputTable(estimate *costs.CostEstimate, parallelOpt *s3.PrefixOptimization, uploadOpt *s3.UploadRecommendations, comparison *costs.ComparisonEstimate, sourcePath string) error {
	// Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("12")). // Blue
		MarginBottom(1)

	fmt.Println(headerStyle.Render(fmt.Sprintf("📊 Cost Estimate for %s (%.1f GB)",
		sourcePath, estimate.TotalSizeGB)))

	// Storage costs table
	fmt.Println("\n💾 Monthly Storage Costs by Class:")
	fmt.Printf("%-20s %12s %12s %12s\n", "Storage Class", "Monthly Cost", "Annual Cost", "Cost per GB")
	fmt.Println(strings.Repeat("-", 60))

	storageOptions := []struct {
		name    string
		monthly float64
	}{
		{"Standard", estimate.StorageCosts.Standard},
		{"Standard-IA", estimate.StorageCosts.StandardIA},
		{"One Zone-IA", estimate.StorageCosts.OneZoneIA},
		{"Intelligent Tiering", estimate.StorageCosts.IntelligentTiering},
		{"Glacier", estimate.StorageCosts.Glacier},
		{"Deep Archive", estimate.StorageCosts.DeepArchive},
	}

	for _, option := range storageOptions {
		annual := option.monthly * 12
		perGB := option.monthly / estimate.TotalSizeGB

		fmt.Printf("%-20s %12s %12s %12s\n",
			option.name,
			fmt.Sprintf("$%.2f", option.monthly),
			fmt.Sprintf("$%.2f", annual),
			fmt.Sprintf("$%.4f", perGB),
		)
	}

	// Transfer costs
	fmt.Printf("\n🌐 Data Transfer Cost: $%.2f (one-time)\n", estimate.TransferCosts.Standard)
	fmt.Printf("📋 Request Costs: $%.2f (one-time)\n", estimate.RequestCosts.Standard)

	// Summary
	fmt.Println("\n📈 Cost Summary:")
	fmt.Printf("%-30s %12s\n", "Cost Type", "Amount")
	fmt.Println(strings.Repeat("-", 45))
	fmt.Printf("%-30s %12s\n", "Upload Cost (one-time)", fmt.Sprintf("$%.2f", estimate.TotalUploadCost))
	fmt.Printf("%-30s %12s\n", "Monthly Storage (optimized)", fmt.Sprintf("$%.2f", estimate.TotalMonthlyCost))
	fmt.Printf("%-30s %12s\n", "Annual Storage (projected)", fmt.Sprintf("$%.2f", estimate.TotalAnnualCost))

	// Recommendations
	if showRecommendations && len(estimate.Recommendations) > 0 {
		fmt.Println("\n💡 Optimization Recommendations:")
		for i, rec := range estimate.Recommendations {
			impactIcon := "🔸"
			switch rec.Impact {
			case "high":
				impactIcon = "🔴"
			case "medium":
				impactIcon = "🟡"
			case "low":
				impactIcon = "🟢"
			}

			fmt.Printf("%s %d. %s\n", impactIcon, i+1, rec.Description)
			fmt.Printf("   💰 Estimated savings: $%.2f/month (%.0f%% confidence)\n",
				rec.EstimatedSavings, rec.Confidence*100)
		}

		totalSavings := 0.0
		for _, rec := range estimate.Recommendations {
			totalSavings += rec.EstimatedSavings
		}

		if totalSavings > 0 {
			fmt.Printf("\n🎯 Total potential savings: $%.2f/month ($%.2f/year)\n",
				totalSavings, totalSavings*12)
		}
	}

	// Parallel Upload Optimization
	if parallelOpt != nil {
		fmt.Println("\n🚀 Parallel Upload Optimization:")
		fmt.Printf("   Recommended Prefixes: %d\n", parallelOpt.RecommendedPrefixes)
		fmt.Printf("   Recommended Concurrency: %d\n", parallelOpt.RecommendedConcurrency)
		fmt.Printf("   Optimal Pattern: %s\n", parallelOpt.OptimalPattern)
		fmt.Printf("   Size Variation: %.2f%%\n", parallelOpt.SizeVariation*100)

		if parallelOpt.RecommendedPrefixes > 1 {
			estimatedSpeedup := float64(parallelOpt.RecommendedPrefixes) * 0.8 // Conservative estimate
			fmt.Printf("   Estimated Speedup: %.1fx\n", estimatedSpeedup)
		}
	}

	// Upload Optimization
	if uploadOpt != nil {
		fmt.Println("\n⚡ Intelligent Upload Optimization:")
		fmt.Printf("   Optimal Chunk Size: %.1f MB\n", float64(uploadOpt.OptimalChunkSize)/(1024*1024))
		fmt.Printf("   Optimal Concurrency: %d\n", uploadOpt.OptimalConcurrency)
		fmt.Printf("   Network Condition: %s\n", uploadOpt.NetworkCondition)
		fmt.Printf("   Estimated Duration: %v\n", uploadOpt.EstimatedDuration.Round(time.Second))
		fmt.Printf("   Confidence Level: %.0f%%\n", uploadOpt.ConfidenceLevel*100)
		fmt.Printf("   📝 %s\n", uploadOpt.Reasoning)
	}

	// Metadata
	fmt.Printf("\n📋 Analysis Details:\n")
	fmt.Printf("   Region: %s\n", estimate.Region)
	fmt.Printf("   Total Size: %s\n", humanize.Bytes(uint64(estimate.TotalSizeGB*1024*1024*1024)))
	fmt.Printf("   Archives: %d\n", estimate.ArchiveCount)
	fmt.Printf("   Calculated: %s\n", estimate.CalculatedAt.Format("2006-01-02 15:04:05 MST"))

	// Issue #169: Show cost comparison if available
	if comparison != nil {
		fmt.Println("\n═══════════════════════════════════════════════════════════")
		fmt.Println("💰 COST COMPARISON: Naive Upload vs CargoShip Chunking")
		fmt.Println("═══════════════════════════════════════════════════════════")

		// File statistics
		fmt.Printf("\n📊 File Statistics:\n")
		fmt.Printf("   Total Files:        %s\n", humanize.Comma(int64(comparison.FileStats.TotalFiles)))
		fmt.Printf("   Total Size:         %.2f GB\n", comparison.FileStats.TotalSizeGB)
		fmt.Printf("   Average File Size:  %.2f KB\n", comparison.FileStats.AverageFileSizeKB)
		fmt.Printf("   Estimated Chunks:   %d\n", comparison.FileStats.EstimatedChunks)

		// Naive upload costs
		fmt.Println("\n📤 Naive Upload (Individual Files):")
		fmt.Printf("   Storage Cost:       $%.2f/month\n", comparison.NaiveUploadCost.TotalMonthlyCost)
		fmt.Printf("   Request Cost:       $%.2f (one-time)\n", comparison.NaiveUploadCost.TotalUploadCost)
		fmt.Printf("   Total Cost:         $%.2f/month\n", comparison.NaiveUploadCost.TotalMonthlyCost+comparison.NaiveUploadCost.TotalUploadCost)

		// CargoShip chunked costs
		fmt.Println("\n📦 CargoShip (Chunked Archives):")
		fmt.Printf("   Storage Cost:       $%.2f/month\n", comparison.ChunkedUploadCost.TotalMonthlyCost)
		fmt.Printf("   Request Cost:       $%.2f (one-time)\n", comparison.ChunkedUploadCost.TotalUploadCost)
		fmt.Printf("   Total Cost:         $%.2f/month\n", comparison.ChunkedUploadCost.TotalMonthlyCost+comparison.ChunkedUploadCost.TotalUploadCost)

		// Savings breakdown
		fmt.Println("\n💵 Savings Breakdown:")
		if comparison.SavingsBreakdown.MinimumSizePenaltySaved > 0 {
			fmt.Printf("   Minimum Size Penalty Eliminated: $%.2f/month\n", comparison.SavingsBreakdown.MinimumSizePenaltySaved)
		}
		if comparison.SavingsBreakdown.RequestCostSaved > 0 {
			fmt.Printf("   Request Cost Savings:             $%.2f (one-time)\n", comparison.SavingsBreakdown.RequestCostSaved)
		}
		if comparison.SavingsBreakdown.MonitoringCostSaved > 0 {
			fmt.Printf("   Monitoring Cost Savings:          $%.2f/month\n", comparison.SavingsBreakdown.MonitoringCostSaved)
		}

		fmt.Printf("\n🎯 Total Monthly Savings:  $%.2f (%.1f%% reduction)\n",
			comparison.SavingsBreakdown.TotalMonthlySavings,
			comparison.SavingsBreakdown.SavingsPercentage)
		fmt.Printf("📅 Annual Savings:         $%.2f\n", comparison.SavingsBreakdown.AnnualSavings)

		// Recommendations
		if len(comparison.Recommendations) > 0 {
			fmt.Println("\n💡 Key Benefits:")
			for _, rec := range comparison.Recommendations {
				fmt.Printf("   %s\n", rec)
			}
		}

		fmt.Println("═══════════════════════════════════════════════════════════")
	}

	return nil
}

func init() {
	// Add estimate command to root (we'll need to add this to root.go)
}
