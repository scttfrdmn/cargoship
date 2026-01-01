package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/charmbracelet/lipgloss"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/aws/costs"
	s3pkg "github.com/scttfrdmn/cargoship/pkg/aws/s3"
)

var (
	analyzeFormat         string
	analyzeRegion         string
	analyzeProfile        string
	showSavings           bool
	analyzeSampleSize     int
	analyzeEnableSampling bool
	analyzeShowProgress   bool
)

// NewAnalyzeCmd creates the analyze command for existing S3 bucket cost analysis
func NewAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze <s3://bucket[/prefix]>",
		Short: "Analyze existing S3 storage and show potential CargoShip savings",
		Long: `Analyze an existing S3 bucket and calculate how much CargoShip could save.

This command scans your existing S3 storage, calculates current costs, and shows
potential savings from re-grating to CargoShip's chunked format.

Examples:
  cargoship analyze s3://my-bucket
  cargoship analyze s3://my-bucket/data --show-savings
  cargoship analyze s3://my-bucket --format json
  cargoship analyze s3://my-bucket --sampling --sample-size 10000
  cargoship analyze s3://my-bucket --region us-west-2`,
		Args: cobra.ExactArgs(1),
		RunE: runAnalyze,
	}

	cmd.Flags().StringVarP(&analyzeFormat, "format", "f", "table", "Output format (table, json)")
	cmd.Flags().StringVar(&analyzeRegion, "region", "", "AWS region (auto-detected from bucket if not specified)")
	cmd.Flags().StringVar(&analyzeProfile, "profile", "", "AWS profile to use")
	cmd.Flags().BoolVar(&showSavings, "show-savings", true, "Show savings comparison with CargoShip re-gration")
	cmd.Flags().IntVar(&analyzeSampleSize, "sample-size", 10000, "Sample size for quick estimates (when --sampling enabled)")
	cmd.Flags().BoolVar(&analyzeEnableSampling, "sampling", false, "Use sampling mode for quick estimates on large buckets")
	cmd.Flags().BoolVar(&analyzeShowProgress, "progress", true, "Show progress during bucket scan")

	return cmd
}

func runAnalyze(cmd *cobra.Command, args []string) error {
	s3URL := args[0]

	// Parse S3 URL
	bucket, prefix, err := s3pkg.ParseS3URL(s3URL)
	if err != nil {
		return fmt.Errorf("invalid S3 URL: %w", err)
	}

	fmt.Printf("Analyzing S3 bucket: s3://%s", bucket)
	if prefix != "" {
		fmt.Printf("/%s", prefix)
	}
	fmt.Println()

	ctx := context.Background()

	// Load AWS configuration
	awsCfg, err := loadAWSConfig(ctx, analyzeProfile, analyzeRegion)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsCfg)

	// Auto-detect region if not specified
	region := analyzeRegion
	if region == "" {
		region, err = s3pkg.GetBucketRegion(ctx, s3Client, bucket)
		if err != nil {
			return fmt.Errorf("failed to detect bucket region: %w", err)
		}
		fmt.Printf("Detected region: %s\n", region)

		// Recreate client with correct region
		awsCfg.Region = region
		s3Client = s3.NewFromConfig(awsCfg)
	}

	// Create bucket scanner
	scanConfig := &s3pkg.BucketScanConfig{
		Bucket:           bucket,
		Prefix:           prefix,
		EnableSampling:   analyzeEnableSampling,
		SampleSize:       analyzeSampleSize,
		Concurrency:      1,
		ProgressInterval: 2 * time.Second,
	}

	scanner := s3pkg.NewBucketScanner(s3Client, scanConfig)

	// Set up progress callback
	if analyzeShowProgress {
		scanner.SetProgressCallback(func(stats *s3pkg.BucketStats) {
			fmt.Printf("\rScanning... Objects: %s | Size: %s",
				humanize.Comma(stats.ObjectCount),
				humanize.Bytes(uint64(stats.TotalSize)))
		})
	}

	// Create cost calculator
	calculator := costs.NewCalculator(region)

	// Create S3 analyzer
	analyzer := costs.NewS3Analyzer(calculator, scanner, region)

	// Analyze costs
	if analyzeShowProgress {
		fmt.Println("Analyzing bucket costs...")
	}

	result, err := analyzer.Analyze(ctx)
	if err != nil {
		return fmt.Errorf("failed to analyze costs: %w", err)
	}

	// Set bucket name and prefix
	result.BucketName = bucket
	result.Prefix = prefix

	// Output results
	switch analyzeFormat {
	case "json":
		return outputAnalysisJSON(result)
	case "table":
		return outputAnalysisTable(result, showSavings)
	default:
		return fmt.Errorf("unsupported format: %s", analyzeFormat)
	}
}

// loadAWSConfig loads AWS configuration
func loadAWSConfig(ctx context.Context, profile, region string) (aws.Config, error) {
	var opts []func(*awsconfig.LoadOptions) error

	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}

	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return cfg, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return cfg, nil
}

// outputAnalysisJSON outputs analysis results in JSON format
func outputAnalysisJSON(result *costs.S3AnalysisResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// outputAnalysisTable outputs analysis results in table format
func outputAnalysisTable(result *costs.S3AnalysisResult, showSavings bool) error {
	// Define styles
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86"))

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("212"))

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	savingsStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("82"))

	warningStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("214"))

	// Header
	fmt.Println()
	fmt.Println(titleStyle.Render("═══════════════════════════════════════════════════════════"))
	fmt.Println(titleStyle.Render("📊 S3 COST ANALYSIS"))
	fmt.Println(titleStyle.Render("═══════════════════════════════════════════════════════════"))
	fmt.Println()

	// Bucket information
	fmt.Printf("%s: %s\n", headerStyle.Render("Bucket"), valueStyle.Render(fmt.Sprintf("s3://%s", result.BucketName)))
	if result.Prefix != "" {
		fmt.Printf("%s: %s\n", headerStyle.Render("Prefix"), valueStyle.Render(result.Prefix))
	}
	fmt.Printf("%s: %s\n", headerStyle.Render("Region"), valueStyle.Render(result.Region))
	if result.IsSampled {
		fmt.Printf("%s: %s (based on %s sample)\n",
			warningStyle.Render("Note"),
			warningStyle.Render("Estimated values"),
			humanize.Comma(int64(result.BucketStats.SampleSize)))
	}
	fmt.Println()

	// Bucket statistics
	fmt.Println(headerStyle.Render("📦 Bucket Statistics:"))
	fmt.Printf("   Total Objects:        %s\n", humanize.Comma(result.BucketStats.ObjectCount))
	fmt.Printf("   Total Size:           %s\n", humanize.Bytes(uint64(result.BucketStats.TotalSize)))
	fmt.Printf("   Average Object Size:  %s\n", humanize.Bytes(uint64(result.BucketStats.AverageSize)))
	fmt.Println()

	// File size distribution
	fmt.Println(headerStyle.Render("📏 Size Distribution:"))
	fmt.Printf("   Small (<1 MB):        %s\n", humanize.Comma(result.BucketStats.SmallFiles))
	fmt.Printf("   Medium (1-100 MB):    %s\n", humanize.Comma(result.BucketStats.MediumFiles))
	fmt.Printf("   Large (100MB-1GB):    %s\n", humanize.Comma(result.BucketStats.LargeFiles))
	fmt.Printf("   Huge (>1 GB):         %s\n", humanize.Comma(result.BucketStats.HugeFiles))
	fmt.Println()

	// Storage class distribution
	if len(result.BucketStats.StorageClassCounts) > 0 {
		fmt.Println(headerStyle.Render("💾 Storage Class Distribution:"))
		for class, count := range result.BucketStats.StorageClassCounts {
			sizeBytes := result.BucketStats.StorageClassSizes[class]
			fmt.Printf("   %-25s %12s  (%s)\n",
				class,
				humanize.Comma(count),
				humanize.Bytes(uint64(sizeBytes)))
		}
		fmt.Println()
	}

	// Current costs
	fmt.Println(titleStyle.Render("💰 CURRENT MONTHLY COSTS"))
	fmt.Println(titleStyle.Render("═══════════════════════════════════════════════════════════"))
	fmt.Println()

	fmt.Printf("   Storage Cost:         $%.2f\n", result.CurrentCosts.StorageCost)
	if result.CurrentCosts.MonitoringFees > 0 {
		fmt.Printf("   Monitoring Fees:      $%.2f  %s\n",
			result.CurrentCosts.MonitoringFees,
			warningStyle.Render("(INTELLIGENT_TIERING)"))
	}
	fmt.Printf("   Request Costs:        $%.2f  (estimated)\n", result.CurrentCosts.RequestCosts)
	fmt.Println()
	fmt.Printf("   %s:  %s\n",
		headerStyle.Render("Total Monthly Cost"),
		valueStyle.Render(fmt.Sprintf("$%.2f", result.CurrentCosts.TotalMonthlyCost)))
	fmt.Printf("   %s: %s\n",
		headerStyle.Render("Annual Cost"),
		valueStyle.Render(fmt.Sprintf("$%.2f", result.CurrentCosts.TotalAnnualCost)))
	fmt.Println()

	// Savings analysis (if requested)
	if showSavings && result.Savings != nil {
		fmt.Println(titleStyle.Render("🎯 CARGOSHIP SAVINGS POTENTIAL"))
		fmt.Println(titleStyle.Render("═══════════════════════════════════════════════════════════"))
		fmt.Println()

		fmt.Println(headerStyle.Render("📦 After CargoShip Re-gration (Estimated):"))
		fmt.Printf("├─ Chunks:                %s (from %s objects)\n",
			humanize.Comma(int64(result.ProjectedCosts.EstimatedChunks)),
			humanize.Comma(result.BucketStats.ObjectCount))
		fmt.Printf("├─ Total Size:            %s (same)\n",
			humanize.Bytes(uint64(result.BucketStats.TotalSize)))
		fmt.Printf("└─ Projected Monthly Cost: %s\n",
			savingsStyle.Render(fmt.Sprintf("$%.2f", result.ProjectedCosts.TotalMonthlyCost)))
		fmt.Printf("   ├─ Storage:            $%.2f (same)\n", result.ProjectedCosts.StorageCost)
		if result.ProjectedCosts.MonitoringFees > 0 {
			reductionPct := 100.0 * (1.0 - result.ProjectedCosts.MonitoringFees/result.CurrentCosts.MonitoringFees)
			fmt.Printf("   ├─ Monitoring fees:    %s  %s\n",
				savingsStyle.Render(fmt.Sprintf("$%.2f", result.ProjectedCosts.MonitoringFees)),
				savingsStyle.Render(fmt.Sprintf("(%.1f%% reduction!)", reductionPct)))
		} else {
			fmt.Printf("   ├─ Monitoring fees:    $%.2f\n", result.ProjectedCosts.MonitoringFees)
		}
		// Show request costs as FREE when amortized (Issue #170 Phase 2)
		if result.ProjectedCosts.RequestCosts < 0.01 {
			fmt.Printf("   └─ Request costs:      %s\n", savingsStyle.Render("FREE (amortized)"))
		} else {
			fmt.Printf("   └─ Request costs:      $%.2f\n", result.ProjectedCosts.RequestCosts)
		}
		fmt.Println()

		// Savings summary (Issue #170 Phase 2: More compact format)
		fmt.Printf("🎯 %s:  %s %s\n",
			savingsStyle.Render("Monthly Savings"),
			savingsStyle.Render(fmt.Sprintf("$%.2f", result.Savings.MonthlySavings)),
			savingsStyle.Render(fmt.Sprintf("(%.1f%% reduction)", result.Savings.SavingsPercentage)))
		fmt.Printf("📅 %s:   %s\n",
			savingsStyle.Render("Annual Savings"),
			savingsStyle.Render(fmt.Sprintf("$%.2f", result.Savings.AnnualSavings)))
		fmt.Println()

		// Savings breakdown
		if result.Savings.MonitoringFeeSavings > 0 || result.Savings.MinimumSizeSavings > 0 {
			fmt.Println(headerStyle.Render("📊 Savings Breakdown:"))
			if result.Savings.MonitoringFeeSavings > 0 {
				fmt.Printf("   Monitoring fees:      $%.2f/month\n", result.Savings.MonitoringFeeSavings)
			}
			if result.Savings.MinimumSizeSavings > 0 {
				fmt.Printf("   Minimum size penalty: $%.2f/month\n", result.Savings.MinimumSizeSavings)
			}
			if result.Savings.RequestCostSavings > 0 {
				fmt.Printf("   Request costs:        $%.2f/month\n", result.Savings.RequestCostSavings)
			}
			fmt.Println()
		}

		// Migration cost and time (Issue #170 Phase 2: More compact + time estimation)
		if result.MigrationCost != nil {
			fmt.Printf("💰 %s:  %s\n",
				headerStyle.Render("One-time Migration Cost"),
				valueStyle.Render(fmt.Sprintf("$%.2f", result.MigrationCost.TotalCost)))

			// Payback period
			if result.Savings.PaybackPeriodDays < 1 {
				fmt.Printf("⏱️  %s: %s\n",
					headerStyle.Render("Payback Period"),
					savingsStyle.Render(fmt.Sprintf("%.1f hours (IMMEDIATE ROI!)", result.Savings.PaybackPeriodDays*24)))
			} else if result.Savings.PaybackPeriodDays < 30 {
				fmt.Printf("⏱️  %s: %s\n",
					headerStyle.Render("Payback Period"),
					savingsStyle.Render(fmt.Sprintf("%.0f days", result.Savings.PaybackPeriodDays)))
			} else {
				fmt.Printf("⏱️  %s: %s\n",
					headerStyle.Render("Payback Period"),
					savingsStyle.Render(fmt.Sprintf("%.1f months", result.Savings.PaybackPeriodDays/30)))
			}

			// Migration time estimation (Issue #170 Phase 2)
			// Assume 100 MB/s throughput (conservative for S3-to-S3)
			sizeGB := float64(result.BucketStats.TotalSize) / (1024 * 1024 * 1024)
			throughputMBps := 100.0
			estimatedHours := (sizeGB * 1024) / (throughputMBps * 3600)
			if estimatedHours < 1 {
				fmt.Printf("🚀 %s: %s\n",
					headerStyle.Render("Estimated Migration Time"),
					valueStyle.Render(fmt.Sprintf("%.0f minutes", estimatedHours*60)))
			} else if estimatedHours < 24 {
				fmt.Printf("🚀 %s: %s\n",
					headerStyle.Render("Estimated Migration Time"),
					valueStyle.Render(fmt.Sprintf("%.1f hours", estimatedHours)))
			} else {
				fmt.Printf("🚀 %s: %s\n",
					headerStyle.Render("Estimated Migration Time"),
					valueStyle.Render(fmt.Sprintf("%.1f days", estimatedHours/24)))
			}
			fmt.Println()
		}
	}

	// Recommendations
	if len(result.Recommendations) > 0 {
		fmt.Println(titleStyle.Render("💡 RECOMMENDATIONS"))
		fmt.Println(titleStyle.Render("═══════════════════════════════════════════════════════════"))
		fmt.Println()
		for _, rec := range result.Recommendations {
			fmt.Printf("   • %s\n", rec)
		}
		fmt.Println()
	}

	return nil
}
