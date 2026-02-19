package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/spf13/cobra"

	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	"github.com/scttfrdmn/cargoship/pkg/manifest"
)

// NewCostCmd creates the 'cost' command for cost management
func NewCostCmd() *cobra.Command {
	var (
		// Common flags
		region     string
		jsonOutput bool

		// Estimate flags
		size         string
		storageClass string
		operation    string

		// Report flags
		period     string
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "cost",
		Short: "Cost management and budget tracking",
		Long: `Cost management and budget tracking for CargoShip uploads.

The cost command provides cost estimation, budget tracking, and pricing information:
- Estimate costs for planned uploads
- View budget status and spending
- Get current AWS pricing for your region

This replaces the standalone 'cargoship-cost' tool with integrated functionality.

Examples:
  # Estimate cost for uploading 100GB
  cargoship cost estimate --size 100GB --region us-west-2

  # Show budget status
  cargoship cost budget

  # Get current pricing
  cargoship cost pricing --region us-west-2

  # Generate cost report
  cargoship cost report --period month
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// If no subcommand, show help
			return cmd.Help()
		},
	}

	// Global flags
	cmd.PersistentFlags().StringVarP(&region, "region", "r", "us-west-2", "AWS region")
	cmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	// Estimate subcommand
	estimateCmd := &cobra.Command{
		Use:   "estimate",
		Short: "Estimate cost for a planned upload",
		Long: `Estimate storage costs before uploading.

Calculates monthly storage costs based on:
- Data size (uncompressed)
- Storage class (STANDARD, INTELLIGENT_TIERING, GLACIER, etc.)
- Region-specific pricing

Examples:
  # Estimate cost for 100GB upload
  cargoship cost estimate --size 100GB

  # Estimate with specific storage class
  cargoship cost estimate --size 500GB --storage-class GLACIER

  # Estimate for different region
  cargoship cost estimate --size 1TB --region eu-west-1
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCostEstimate(cmd.Context(), region, size, storageClass, operation, jsonOutput)
		},
	}
	estimateCmd.Flags().StringVar(&size, "size", "", "Data size (e.g., 100GB, 500MB, 1TB) (required)")
	estimateCmd.Flags().StringVar(&storageClass, "storage-class", "STANDARD", "Storage class (STANDARD, STANDARD_IA, GLACIER, etc.)")
	estimateCmd.Flags().StringVar(&operation, "operation", "upload", "Operation type (upload, download)")
	if err := estimateCmd.MarkFlagRequired("size"); err != nil {
		panic(fmt.Sprintf("failed to mark size flag as required: %v", err))
	}

	// Upload subcommand - Show actual costs for specific uploads
	uploadCmd := &cobra.Command{
		Use:   "upload",
		Short: "Show actual cost for a specific upload",
		Long: `Display actual storage costs for a CargoShip upload.

Query the manifest to calculate real costs based on:
- Actual compressed size stored in S3
- Storage duration (from CreatedAt timestamp)
- Region and storage class
- Compression savings achieved

Examples:
  # Show cost for specific upload
  cargoship cost upload --bucket my-bucket --upload-id 20231208-123456-abcd1234

  # Show cost with compression ROI details
  cargoship cost upload --bucket my-bucket --upload-id xxx --show-savings

  # JSON output
  cargoship cost upload --bucket my-bucket --upload-id xxx --json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			bucket, _ := cmd.Flags().GetString("bucket")
			prefix, _ := cmd.Flags().GetString("prefix")
			uploadID, _ := cmd.Flags().GetString("upload-id")
			showSavings, _ := cmd.Flags().GetBool("show-savings")
			return runCostUpload(cmd.Context(), region, bucket, prefix, uploadID, showSavings, jsonOutput)
		},
	}
	uploadCmd.Flags().String("bucket", "", "S3 bucket name (required)")
	uploadCmd.Flags().String("prefix", "cargoship", "S3 prefix")
	uploadCmd.Flags().String("upload-id", "", "Upload ID (required)")
	uploadCmd.Flags().Bool("show-savings", true, "Show compression savings")
	if err := uploadCmd.MarkFlagRequired("bucket"); err != nil {
		panic(fmt.Sprintf("failed to mark bucket flag as required: %v", err))
	}
	if err := uploadCmd.MarkFlagRequired("upload-id"); err != nil {
		panic(fmt.Sprintf("failed to mark upload-id flag as required: %v", err))
	}

	// Budget subcommand
	// Budget subcommand - replaced with comprehensive project budget management
	// The NewBudgetCmd() provides full budget management with:
	// - status: Show budget status for a project
	// - set: Set cost budget and volume quota for a project
	// - list: List all configured project budgets
	// - remove: Remove project budget configuration
	budgetCmd := NewBudgetCmd()

	// Pricing subcommand
	pricingCmd := &cobra.Command{
		Use:   "pricing",
		Short: "Show current AWS pricing",
		Long: `Display current AWS S3 pricing for your region.

Shows pricing for:
- Storage (per GB per month) for all storage classes
- Request costs (PUT, GET, etc.)
- Data transfer costs

Examples:
  # Show pricing for default region
  cargoship cost pricing

  # Show pricing for specific region
  cargoship cost pricing --region eu-west-1

  # Pricing as JSON
  cargoship cost pricing --json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPricing(cmd.Context(), region, jsonOutput)
		},
	}

	// Report subcommand
	reportCmd := &cobra.Command{
		Use:   "report",
		Short: "Generate cost report",
		Long: `Generate detailed cost report for a time period or compliance report.

Standard mode (default):
  Shows total costs, breakdown by service/region, trends, and recommendations.

Compliance mode (--format compliance):
  Generates an NSF or NIH data-management compliance report for a specific
  budget/project, including data provenance, reproducibility info, and DMP.

Examples:
  # Monthly report
  cargoship cost report --period month

  # Export report to file
  cargoship cost report --period month --output report.json

  # NSF compliance report
  cargoship cost report --budget my-project-id --grant NSF-2024-12345 --format compliance

  # NIH compliance report (text)
  cargoship cost report --budget my-project-id --grant R01-GM123456 --format compliance --agency NIH --text
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			budgetFlag, _ := cmd.Flags().GetString("budget")
			grantFlag, _ := cmd.Flags().GetString("grant")
			agencyFlag, _ := cmd.Flags().GetString("agency")
			textFlag, _ := cmd.Flags().GetBool("text")
			formatFlag, _ := cmd.Flags().GetString("format")

			if formatFlag == "compliance" || budgetFlag != "" {
				return runComplianceReport(cmd.Context(), region, budgetFlag, grantFlag, agencyFlag, outputFile, jsonOutput || !textFlag)
			}
			return runReport(cmd.Context(), region, period, outputFile, jsonOutput)
		},
	}
	reportCmd.Flags().StringVar(&period, "period", "month", "Report period (today, week, month, last_month)")
	reportCmd.Flags().StringVar(&outputFile, "output", "", "Output file path (default: stdout)")
	reportCmd.Flags().String("budget", "", "Budget/project ID for compliance report")
	reportCmd.Flags().String("grant", "", "Grant/award number (e.g., NSF-2024-12345, R01-GM123456)")
	reportCmd.Flags().String("agency", "NSF", "Funding agency for compliance report (NSF or NIH)")
	reportCmd.Flags().String("format", "", "Output format: compliance (enables compliance report mode)")
	reportCmd.Flags().Bool("text", false, "Render compliance report as human-readable text (default: JSON)")

	// Projects subcommand (Issue #147 Phase 2)
	projectsCmd := &cobra.Command{
		Use:   "projects",
		Short: "List all projects with cost records",
		Long: `List all projects (manifest upload IDs) that have associated cost records.

Projects are identified by their manifest upload IDs (e.g., 20251206-abc123).
Each upload to S3 creates a unique project that can be tracked separately.

Examples:
  # List all projects
  cargoship cost projects

  # List projects as JSON
  cargoship cost projects --json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectsList(cmd.Context(), region, jsonOutput)
		},
	}

	// Project subcommand (Issue #147 Phase 2)
	projectCmd := &cobra.Command{
		Use:   "project PROJECT_ID",
		Short: "Show costs for a specific project",
		Long: `Show detailed cost information for a specific project (manifest upload ID).

Displays:
- Total costs and savings for the project
- Total files and data size uploaded
- Cost breakdown by region and storage class
- First and last upload timestamps
- Average cost per GB

Examples:
  # Show costs for specific project
  cargoship cost project 20251206-abc123

  # Show project costs for specific period
  cargoship cost project 20251206-abc123 --period month

  # Project costs as JSON
  cargoship cost project 20251206-abc123 --json
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProjectSummary(cmd.Context(), region, args[0], period, jsonOutput)
		},
	}
	projectCmd.Flags().StringVar(&period, "period", "all", "Report period (all, today, week, month, last_month)")

	// Forecast subcommand (Issue #147 Phase 6)
	forecastCmd := &cobra.Command{
		Use:   "forecast [PROJECT_ID]",
		Short: "Generate cost forecasts with ML-based projections",
		Long: `Generate cost forecasts using multiple forecasting models.

Predicts future costs based on historical spending patterns using:
- Linear regression (best for stable/trending patterns)
- Exponential smoothing (for seasonal patterns)
- Moving average (for smoothing volatility)
- Ensemble model (combines all models)

Displays:
- Predicted costs at 7, 14, 30, 60, 90 days
- Confidence intervals (90%, 95%, 99%)
- Model accuracy metrics (R², MAE, RMSE)
- Daily cost forecasts

Examples:
  # Generate forecast for all projects (global)
  cargoship cost forecast

  # Generate forecast for specific project
  cargoship cost forecast 20251206-abc123

  # Forecast with specific model
  cargoship cost forecast --model linear

  # Forecast as JSON
  cargoship cost forecast --json
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := ""
			if len(args) > 0 {
				projectID = args[0]
			}
			model, _ := cmd.Flags().GetString("model")
			days, _ := cmd.Flags().GetInt("days")
			return runForecast(cmd.Context(), region, projectID, model, days, jsonOutput)
		},
	}
	forecastCmd.Flags().String("model", "linear", "Forecasting model (linear, exponential, moving_average, ensemble)")
	forecastCmd.Flags().Int("days", 90, "Number of days to forecast (7-90)")

	// Burnrate subcommand (Issue #147 Phase 6)
	burnrateCmd := &cobra.Command{
		Use:   "burnrate [PROJECT_ID]",
		Short: "Analyze burn rate with trend detection",
		Long: `Analyze current and historical burn rates with trend detection.

Provides detailed burn rate metrics:
- Current rates (daily, weekly, monthly)
- Historical statistics (average, min, max, std dev, volatility)
- Trend detection (increasing/decreasing/stable) with strength
- Acceleration rate (change in burn rate per day)
- Predicted future burn rates (30/60/90 days)
- Confidence intervals for predictions

Examples:
  # Analyze burn rate for all projects (global)
  cargoship cost burnrate

  # Analyze burn rate for specific project
  cargoship cost burnrate 20251206-abc123

  # Analyze last 60 days
  cargoship cost burnrate --days 60

  # Burn rate as JSON
  cargoship cost burnrate --json
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := ""
			if len(args) > 0 {
				projectID = args[0]
			}
			days, _ := cmd.Flags().GetInt("days")
			return runBurnrate(cmd.Context(), region, projectID, days, jsonOutput)
		},
	}
	burnrateCmd.Flags().Int("days", 90, "Number of days of historical data to analyze (7-365)")

	// Exhaustion subcommand (Issue #147 Phase 6)
	exhaustionCmd := &cobra.Command{
		Use:   "exhaustion [PROJECT_ID] --budget AMOUNT",
		Short: "Predict when budget will be exhausted",
		Long: `Predict when a budget will be exhausted based on current spending patterns.

Calculates:
- Exact date when budget will run out
- Days until exhaustion
- Probability of exhaustion (based on confidence intervals)
- Budget usage forecast with confidence bounds

Handles edge cases:
- Budget already exhausted (today)
- Budget never exhausts within 90 days
- High/low confidence scenarios

Examples:
  # Predict exhaustion for $1000 budget (global)
  cargoship cost exhaustion --budget 1000

  # Predict for specific project
  cargoship cost exhaustion 20251206-abc123 --budget 500

  # Include current spending
  cargoship cost exhaustion --budget 1000 --spent 400

  # Exhaustion prediction as JSON
  cargoship cost exhaustion --budget 1000 --json
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectID := ""
			if len(args) > 0 {
				projectID = args[0]
			}
			budget, _ := cmd.Flags().GetFloat64("budget")
			spent, _ := cmd.Flags().GetFloat64("spent")
			if budget <= 0 {
				return fmt.Errorf("--budget is required and must be > 0")
			}
			return runExhaustion(cmd.Context(), region, projectID, budget, spent, jsonOutput)
		},
	}
	exhaustionCmd.Flags().Float64("budget", 0, "Total budget amount (required)")
	exhaustionCmd.Flags().Float64("spent", 0, "Amount already spent (default: calculated from cost records)")
	_ = exhaustionCmd.MarkFlagRequired("budget")

	// Benchmark Compare subcommand (Issue #165)
	benchmarkCmd := &cobra.Command{
		Use:   "benchmark-compare",
		Short: "Compare CargoShip costs vs competitors for benchmarking",
		Long: `Calculate and compare costs for benchmark scenarios.

Shows CargoShip's cost advantages:
- Compression savings (20-70% data reduction)
- Intelligent chunking (50% fewer requests)
- Storage tier optimization (30-60% cost reduction)
- Deduplication (variable savings)

Output format is JSON for easy integration with benchmark scripts.

Examples:
  # Compare CargoShip (3:1 compression) vs competitor (no compression)
  cargoship cost benchmark-compare --size 100GB --files 10000 \
    --compression-ratio 3.0 --storage-class GLACIER

  # Compare with deduplication
  cargoship cost benchmark-compare --size 100GB --files 10000 \
    --compression-ratio 2.0 --dedup-ratio 2.0

  # Competitor cost only
  cargoship cost benchmark-compare --tool s5cmd --size 100GB --files 10000
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			tool, _ := cmd.Flags().GetString("tool")
			sizeGB, _ := cmd.Flags().GetFloat64("size-gb")
			fileCount, _ := cmd.Flags().GetInt64("files")
			compressionRatio, _ := cmd.Flags().GetFloat64("compression-ratio")
			dedupRatio, _ := cmd.Flags().GetFloat64("dedup-ratio")
			storageClassStr, _ := cmd.Flags().GetString("storage-class")
			showChart, _ := cmd.Flags().GetBool("chart")

			// Parse storage class
			storageClass, err := parseStorageClass(storageClassStr)
			if err != nil {
				return fmt.Errorf("invalid storage class: %w", err)
			}

			return runBenchmarkCompare(cmd.Context(), region, tool, sizeGB, fileCount,
				compressionRatio, dedupRatio, storageClass, showChart)
		},
	}
	benchmarkCmd.Flags().String("tool", "", "Tool name (s5cmd, rclone, aws-cli, cargoship)")
	benchmarkCmd.Flags().Float64("size-gb", 0, "Data size in GB (required)")
	benchmarkCmd.Flags().Int64("files", 0, "Number of files (required)")
	benchmarkCmd.Flags().Float64("compression-ratio", 1.0, "Compression ratio (e.g., 3.0 for 3:1)")
	benchmarkCmd.Flags().Float64("dedup-ratio", 1.0, "Deduplication ratio (e.g., 2.0 for 2:1)")
	benchmarkCmd.Flags().String("storage-class", "STANDARD", "Storage class")
	benchmarkCmd.Flags().Bool("chart", false, "Display ASCII cost comparison charts")
	_ = benchmarkCmd.MarkFlagRequired("size-gb")
	_ = benchmarkCmd.MarkFlagRequired("files")

	// Summary subcommand — DVC stage / git-commit cost aggregation (Issue #186)
	summaryCmd := &cobra.Command{
		Use:   "summary",
		Short: "Summarize costs by DVC stage or git commit",
		Long: `Aggregate recorded costs by DVC pipeline stage or git commit.

Requires cost records that were tagged with DVC provenance information
(populated automatically when --dvc-stage is passed to 'cargoship upload').

Examples:
  # Summarise costs for a specific DVC stage
  cargoship cost summary --by-dvc-stage preprocess

  # List all records for a git commit
  cargoship cost summary --git-commit abc1234
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			dvcStageFlag, _ := cmd.Flags().GetString("by-dvc-stage")
			gitCommitFlag, _ := cmd.Flags().GetString("git-commit")

			if dvcStageFlag == "" && gitCommitFlag == "" {
				return fmt.Errorf("one of --by-dvc-stage or --git-commit is required")
			}
			if dvcStageFlag != "" && gitCommitFlag != "" {
				return fmt.Errorf("--by-dvc-stage and --git-commit are mutually exclusive")
			}

			return runCostSummary(cmd.Context(), region, dvcStageFlag, gitCommitFlag, jsonOutput)
		},
	}
	summaryCmd.Flags().String("by-dvc-stage", "", "Aggregate costs for this DVC pipeline stage")
	summaryCmd.Flags().String("git-commit", "", "List costs tagged with this git commit SHA")

	// Add subcommands
	cmd.AddCommand(estimateCmd, uploadCmd, budgetCmd, pricingCmd, reportCmd, projectsCmd, projectCmd, forecastCmd, burnrateCmd, exhaustionCmd, benchmarkCmd, summaryCmd)

	return cmd
}

func runCostEstimate(ctx context.Context, region, sizeStr, storageClassStr, operation string, jsonOutput bool) error {
	// Parse size
	sizeBytes, err := parseSize(sizeStr)
	if err != nil {
		return fmt.Errorf("invalid size format: %w", err)
	}
	sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)

	// Parse storage class
	storageClass, err := parseStorageClass(storageClassStr)
	if err != nil {
		return fmt.Errorf("invalid storage class: %w", err)
	}

	// Load AWS config
	awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, region)
	if err != nil {
		return err
	}

	// Create cost manager
	costMgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create cost manager: %w", err)
	}

	// Get estimate
	estimate, err := costMgr.EstimateOperationCost(ctx, operation, sizeGB, storageClass, region)
	if err != nil {
		return fmt.Errorf("failed to estimate cost: %w", err)
	}

	// Display results
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(estimate)
	}

	// Human-readable output
	fmt.Printf("💰 %s\n", makeHeader("Cost Estimate"))
	fmt.Printf("   Size:             %.2f GB (%.2f GB compressed at 22%%)\n", sizeGB, sizeGB*0.22)
	fmt.Printf("   Storage Class:    %s\n", storageClassStr)
	fmt.Printf("   Region:           %s\n", region)
	fmt.Printf("   Operation:        %s\n", operation)
	fmt.Printf("   Currency:         %s\n", estimate.Currency)
	fmt.Println()

	fmt.Printf("📊 %s\n", makeHeader("Monthly Cost Breakdown"))
	fmt.Printf("   Storage Cost:     $%.4f/month\n", estimate.StorageCost)
	fmt.Printf("   Request Cost:     $%.4f\n", estimate.RequestCost)
	fmt.Printf("   Data Transfer:    $%.4f\n", estimate.DataTransferCost)
	fmt.Printf("   ─────────────────────────────\n")
	fmt.Printf("   Total Cost:       $%.4f/month\n", estimate.TotalCost)
	fmt.Println()

	if estimate.Discounts.TotalDiscount > 0 {
		fmt.Printf("🎁 %s\n", makeHeader("Discounts Applied"))
		fmt.Printf("   Original Cost:    $%.4f\n", estimate.Discounts.OriginalCost)
		fmt.Printf("   Total Discount:   $%.4f\n", estimate.Discounts.TotalDiscount)
		fmt.Printf("   Final Cost:       $%.4f\n", estimate.Discounts.DiscountedCost)
		fmt.Println()
	}

	// Show compression savings estimate
	uncompressedCost := estimate.TotalCost / 0.22 // Assuming 22% compression ratio
	savings := uncompressedCost - estimate.TotalCost
	if savings > 0 {
		savingsPercent := (savings / uncompressedCost) * 100
		fmt.Printf("🗜️  %s\n", makeHeader("Estimated Compression Savings"))
		fmt.Printf("   Uncompressed Cost:   $%.4f/month\n", uncompressedCost)
		fmt.Printf("   Compressed Cost:     $%.4f/month\n", estimate.TotalCost)
		fmt.Printf("   Monthly Savings:     $%.4f (%.1f%% reduction)\n", savings, savingsPercent)
		fmt.Println()
	}

	return nil
}

func runPricing(ctx context.Context, region string, jsonOutput bool) error {
	// Load AWS config
	awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, region)
	if err != nil {
		return err
	}

	// Create cost manager
	costMgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create cost manager: %w", err)
	}

	// Get pricing
	pricing, err := costMgr.GetCurrentPricing(ctx, region)
	if err != nil {
		return fmt.Errorf("failed to get pricing: %w", err)
	}

	// Display results
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(pricing)
	}

	// Human-readable output
	fmt.Printf("💵 %s - %s\n", makeHeader("Current AWS Pricing"), region)
	fmt.Printf("   Currency:         %s\n", pricing["currency"])
	fmt.Printf("   Source:           %s\n", pricing["source"])
	fmt.Printf("   Last Updated:     %s\n", pricing["last_updated"])
	fmt.Println()

	if storage, ok := pricing["storage_per_gb_month"].(map[string]float64); ok {
		fmt.Printf("📦 %s\n", makeHeader("Storage Pricing (per GB per month)"))
		for class, price := range storage {
			fmt.Printf("   %-25s $%.6f\n", class, price)
		}
		fmt.Println()
	}

	if requests, ok := pricing["requests"].(map[string]float64); ok {
		fmt.Printf("🔄 %s\n", makeHeader("Request Pricing"))
		for reqType, price := range requests {
			fmt.Printf("   %-25s $%.6f\n", reqType, price)
		}
		fmt.Println()
	}

	return nil
}

func runReport(ctx context.Context, region, period, outputFile string, jsonOutput bool) error {
	// Load AWS config
	awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, region)
	if err != nil {
		return err
	}

	// Create cost manager
	costMgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create cost manager: %w", err)
	}

	// Generate report
	summary, err := costMgr.GenerateCostReport(ctx, period)
	if err != nil {
		return fmt.Errorf("failed to generate report: %w", err)
	}

	// Export to file if specified
	if outputFile != "" {
		format := "json"
		if strings.HasSuffix(outputFile, ".csv") {
			format = "csv"
		}
		return costMgr.ExportCostReport(ctx, summary, format, outputFile)
	}

	// Display results
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(summary)
	}

	// Human-readable output
	fmt.Printf("📈 %s - %s\n", makeHeader("Cost Report"), summary.Period)
	fmt.Printf("   Total Cost:       $%.2f %s\n", summary.TotalCost, summary.Currency)
	fmt.Printf("   Total Savings:    $%.2f %s\n", summary.TotalSavings, summary.Currency)
	fmt.Println()

	if len(summary.ByService) > 0 {
		fmt.Printf("🔧 %s\n", makeHeader("By Service"))
		for service, cost := range summary.ByService {
			fmt.Printf("   %-20s $%.2f\n", service, cost)
		}
		fmt.Println()
	}

	if len(summary.ByRegion) > 0 {
		fmt.Printf("🌍 %s\n", makeHeader("By Region"))
		for reg, cost := range summary.ByRegion {
			fmt.Printf("   %-20s $%.2f\n", reg, cost)
		}
		fmt.Println()
	}

	fmt.Printf("📊 %s\n", makeHeader("Trends"))
	fmt.Printf("   Daily Average:       $%.2f\n", summary.Trends.DailyAverage)
	fmt.Printf("   Weekly Average:      $%.2f\n", summary.Trends.WeeklyAverage)
	fmt.Printf("   Monthly Projection:  $%.2f\n", summary.Trends.MonthlyProjection)
	fmt.Printf("   Cost per GB:         $%.4f\n", summary.Trends.CostPerGB)
	fmt.Println()

	if len(summary.Recommendations) > 0 {
		fmt.Printf("💡 %s\n", makeHeader("Recommendations"))
		for _, rec := range summary.Recommendations {
			priority := strings.ToUpper(rec.Priority)
			fmt.Printf("   [%s] %s\n", priority, rec.Description)
			fmt.Printf("        Potential Saving: $%.2f\n", rec.PotentialSaving)
		}
		fmt.Println()
	}

	return nil
}

// Helper functions

func loadAWSConfigForCost(ctx context.Context, region string) (aws.Config, *cargoconfig.AWSConfig, error) {
	// Load CargoShip config (uses default if no config file)
	cargoConfig := cargoconfig.DefaultAWSConfig()
	if region != "" {
		cargoConfig.Region = region
	}

	// Load AWS SDK config
	awsCfg, err := cargoconfig.LoadAWSConfig(ctx, cargoConfig.Profile, cargoConfig.Region)
	if err != nil {
		return aws.Config{}, nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return awsCfg, cargoConfig, nil
}

func parseSize(sizeStr string) (int64, error) {
	sizeStr = strings.ToUpper(strings.TrimSpace(sizeStr))

	var multiplier int64 = 1
	var numStr string

	if strings.HasSuffix(sizeStr, "TB") {
		multiplier = 1024 * 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(sizeStr, "TB")
	} else if strings.HasSuffix(sizeStr, "GB") {
		multiplier = 1024 * 1024 * 1024
		numStr = strings.TrimSuffix(sizeStr, "GB")
	} else if strings.HasSuffix(sizeStr, "MB") {
		multiplier = 1024 * 1024
		numStr = strings.TrimSuffix(sizeStr, "MB")
	} else if strings.HasSuffix(sizeStr, "KB") {
		multiplier = 1024
		numStr = strings.TrimSuffix(sizeStr, "KB")
	} else if strings.HasSuffix(sizeStr, "B") {
		multiplier = 1
		numStr = strings.TrimSuffix(sizeStr, "B")
	} else {
		numStr = sizeStr
	}

	num, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, err
	}

	return int64(num * float64(multiplier)), nil
}

func parseStorageClass(class string) (cargoconfig.StorageClass, error) {
	switch strings.ToUpper(class) {
	case "STANDARD":
		return cargoconfig.StorageClassStandard, nil
	case "STANDARD_IA":
		return cargoconfig.StorageClassStandardIA, nil
	case "ONEZONE_IA":
		return cargoconfig.StorageClassOneZoneIA, nil
	case "INTELLIGENT_TIERING":
		return cargoconfig.StorageClassIntelligentTiering, nil
	case "GLACIER":
		return cargoconfig.StorageClassGlacier, nil
	case "DEEP_ARCHIVE":
		return cargoconfig.StorageClassDeepArchive, nil
	default:
		return "", fmt.Errorf("unknown storage class: %s", class)
	}
}

func runCostUpload(ctx context.Context, region, bucket, prefix, uploadID string, showSavings, jsonOutput bool) error {
	// Download manifest from S3
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	s3Client := s3.NewFromConfig(awsCfg)

	// Build manifest key
	manifestKey := fmt.Sprintf("%s/uploads/%s/manifest.json.gz", prefix, uploadID)

	// Download manifest
	getObjectInput := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(manifestKey),
	}

	result, err := s3Client.GetObject(ctx, getObjectInput)
	if err != nil {
		return fmt.Errorf("failed to download manifest from S3: %w", err)
	}
	defer func() {
		if cerr := result.Body.Close(); cerr != nil {
			slog.Warn("failed to close manifest body", "error", cerr)
		}
	}()

	// Read and decompress manifest
	manifestBytes, err := io.ReadAll(result.Body)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	m, err := manifest.FromJSONCompressed(manifestBytes)
	if err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}

	// Calculate total compressed size
	var totalCompressedSize int64
	for _, shard := range m.Shards {
		totalCompressedSize += shard.CompressedSize
	}

	// Calculate storage duration
	now := time.Now()
	durationDays := now.Sub(m.CreatedAt).Hours() / 24
	durationMonths := durationDays / 30.0

	// Calculate costs (simplified - using STANDARD pricing)
	// S3 STANDARD: $0.023 per GB/month
	pricePerGBMonth := 0.023
	compressedSizeGB := float64(totalCompressedSize) / (1024 * 1024 * 1024)
	uncompressedSizeGB := float64(m.TotalBytes) / (1024 * 1024 * 1024)

	monthlyCost := compressedSizeGB * pricePerGBMonth
	totalSpent := monthlyCost * durationMonths

	// Calculate compression savings
	uncompressedCost := uncompressedSizeGB * pricePerGBMonth
	savingsPerMonth := uncompressedCost - monthlyCost
	savingsPercent := (savingsPerMonth / uncompressedCost) * 100
	effectiveCostPerGB := monthlyCost / uncompressedSizeGB

	if jsonOutput {
		output := map[string]interface{}{
			"upload_id":             m.UploadID,
			"region":                m.Region,
			"created_at":            m.CreatedAt,
			"duration_days":         durationDays,
			"uncompressed_size_gb":  uncompressedSizeGB,
			"compressed_size_gb":    compressedSizeGB,
			"compression_ratio":     m.CompressionRatio,
			"monthly_cost":          monthlyCost,
			"total_spent":           totalSpent,
			"uncompressed_cost":     uncompressedCost,
			"savings_per_month":     savingsPerMonth,
			"savings_percent":       savingsPercent,
			"effective_cost_per_gb": effectiveCostPerGB,
		}
		jsonBytes, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON: %w", err)
		}
		fmt.Println(string(jsonBytes))
		return nil
	}

	// Human-readable output
	fmt.Printf("%s\n", makeHeader("📦 Upload Cost Analysis"))
	fmt.Printf("   Upload ID:           %s\n", m.UploadID)
	fmt.Printf("   Region:              %s\n", m.Region)
	fmt.Printf("   Created:             %s\n", m.CreatedAt.Format("2006-01-02"))
	fmt.Printf("   Duration:            %.0f days\n", durationDays)
	fmt.Println()

	fmt.Printf("%s\n", makeHeader("💰 Storage Costs"))
	fmt.Printf("   Monthly Cost:        $%.2f/month\n", monthlyCost)
	fmt.Printf("   Total Spent:         $%.2f (%.0f days)\n", totalSpent, durationDays)
	fmt.Println()

	if showSavings {
		fmt.Printf("%s\n", makeHeader("🗜️  Compression Savings"))
		fmt.Printf("   Actual Data:         %.2f GB\n", uncompressedSizeGB)
		fmt.Printf("   Stored As:           %.2f GB (%.1f%% compression)\n",
			compressedSizeGB, m.CompressionRatio*100)
		fmt.Printf("   Uncompressed Cost:   $%.2f/month (if stored uncompressed)\n", uncompressedCost)
		fmt.Printf("   Savings:             $%.2f/month (%.1f%% reduction)\n",
			savingsPerMonth, savingsPercent)
		fmt.Printf("   Effective Cost:      $%.5f/GB (vs $%.3f/GB STANDARD)\n",
			effectiveCostPerGB, pricePerGBMonth)
		fmt.Println()
	}

	return nil
}

// Project-based cost query functions (Issue #147 Phase 2)

func runProjectsList(ctx context.Context, region string, jsonOutput bool) error {
	// Load AWS config
	awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, region)
	if err != nil {
		return err
	}

	// Create cost manager
	costMgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create cost manager: %w", err)
	}

	// Get list of projects
	projects := costMgr.GetReporter().ListProjects()

	// Display results
	if jsonOutput {
		output := map[string]interface{}{
			"projects": projects,
			"count":    len(projects),
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}

	// Human-readable output
	if len(projects) == 0 {
		fmt.Printf("📦 No projects found\n")
		fmt.Printf("   Projects are created automatically when you upload data to S3.\n")
		fmt.Printf("   Each upload has a unique project ID (manifest upload ID).\n")
		return nil
	}

	fmt.Printf("📦 %s\n", makeHeader(fmt.Sprintf("%d Projects with Cost Records", len(projects))))
	fmt.Println()

	for i, projectID := range projects {
		projectCost := costMgr.GetReporter().GetProjectCosts(projectID)
		fmt.Printf("   %2d. %s\n", i+1, projectID)
		fmt.Printf("       Total Cost: $%.4f\n", projectCost)
	}
	fmt.Println()
	fmt.Printf("💡 Tip: View details for a project with: cargoship cost project <PROJECT_ID>\n")
	fmt.Println()

	return nil
}

func runProjectSummary(ctx context.Context, region, projectID, period string, jsonOutput bool) error {
	// Load AWS config
	awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, region)
	if err != nil {
		return err
	}

	// Create cost manager
	costMgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create cost manager: %w", err)
	}

	reporter := costMgr.GetReporter()

	// Get project summary
	summary, err := reporter.GetProjectSummary(projectID)
	if err != nil {
		return fmt.Errorf("failed to get project summary: %w", err)
	}

	// If period specified and not "all", filter by period
	var periodCost float64
	if period != "all" {
		periodCost, err = reporter.GetProjectCostsByPeriod(projectID, period)
		if err != nil {
			return fmt.Errorf("failed to get project costs for period: %w", err)
		}
	}

	// Display results
	if jsonOutput {
		output := summary
		if period != "all" {
			// Create a modified output with period costs
			outputMap := map[string]interface{}{
				"project_id":          summary.ProjectID,
				"period":              period,
				"period_cost":         periodCost,
				"total_cost":          summary.TotalCost,
				"total_savings":       summary.TotalSavings,
				"total_files":         summary.TotalFiles,
				"total_size_gb":       summary.TotalSizeGB,
				"currency":            summary.Currency,
				"first_upload":        summary.FirstUpload,
				"last_upload":         summary.LastUpload,
				"by_region":           summary.ByRegion,
				"by_storage_class":    summary.ByStorageClass,
				"average_cost_per_gb": summary.AverageCostPerGB,
			}
			encoder := json.NewEncoder(os.Stdout)
			encoder.SetIndent("", "  ")
			return encoder.Encode(outputMap)
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(output)
	}

	// Human-readable output
	fmt.Printf("📊 %s\n", makeHeader(fmt.Sprintf("Project: %s", projectID)))
	fmt.Println()

	// Period-specific costs
	if period != "all" {
		fmt.Printf("📅 %s\n", makeHeader(fmt.Sprintf("Period: %s", period)))
		fmt.Printf("   Period Cost:      $%.4f %s\n", periodCost, summary.Currency)
		fmt.Println()
	}

	// Overall project stats
	fmt.Printf("💰 %s\n", makeHeader("Overall Project Costs"))
	fmt.Printf("   Total Cost:       $%.4f %s\n", summary.TotalCost, summary.Currency)
	fmt.Printf("   Total Savings:    $%.4f %s\n", summary.TotalSavings, summary.Currency)
	fmt.Printf("   Effective Cost:   $%.4f %s\n", summary.TotalCost-summary.TotalSavings, summary.Currency)
	fmt.Println()

	// Data volume
	fmt.Printf("📦 %s\n", makeHeader("Data Volume"))
	fmt.Printf("   Total Files:      %d\n", summary.TotalFiles)
	fmt.Printf("   Total Size:       %.2f GB\n", summary.TotalSizeGB)
	fmt.Printf("   Avg Cost/GB:      $%.6f\n", summary.AverageCostPerGB)
	fmt.Println()

	// Timeline
	fmt.Printf("📅 %s\n", makeHeader("Timeline"))
	fmt.Printf("   First Upload:     %s\n", summary.FirstUpload.Format("2006-01-02 15:04:05 MST"))
	fmt.Printf("   Last Upload:      %s\n", summary.LastUpload.Format("2006-01-02 15:04:05 MST"))
	duration := summary.LastUpload.Sub(summary.FirstUpload)
	if duration > 0 {
		fmt.Printf("   Duration:         %s\n", duration.Round(time.Second))
	}
	fmt.Println()

	// Regional breakdown
	if len(summary.ByRegion) > 0 {
		fmt.Printf("🌍 %s\n", makeHeader("Cost by Region"))
		for region, regionCost := range summary.ByRegion {
			percent := (regionCost / summary.TotalCost) * 100
			fmt.Printf("   %-20s $%.4f (%.1f%%)\n", region, regionCost, percent)
		}
		fmt.Println()
	}

	// Storage class breakdown
	if len(summary.ByStorageClass) > 0 {
		fmt.Printf("💾 %s\n", makeHeader("Cost by Storage Class"))
		for class, classCost := range summary.ByStorageClass {
			percent := (classCost / summary.TotalCost) * 100
			fmt.Printf("   %-25s $%.4f (%.1f%%)\n", class, classCost, percent)
		}
		fmt.Println()
	}

	return nil
}

// runForecast generates cost forecasts using the forecast engine
func runForecast(ctx context.Context, region, projectID, modelStr string, days int, jsonOutput bool) error {
	// Load cost manager
	costMgr, err := loadCostManagerWithRegion(ctx, region)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	// Create forecast engine
	engine := cost.NewForecastEngine(costMgr.GetReporter())

	// Parse forecast model
	var model cost.ForecastModel
	switch strings.ToLower(modelStr) {
	case "linear":
		model = cost.ForecastModelLinear
	case "exponential":
		model = cost.ForecastModelExponential
	case "moving_average":
		model = cost.ForecastModelMovingAverage
	case "ensemble":
		model = cost.ForecastModelEnsemble
	default:
		return fmt.Errorf("invalid forecast model: %s (choose: linear, exponential, moving_average, ensemble)", modelStr)
	}

	// Validate days
	if days < 7 || days > 90 {
		return fmt.Errorf("days must be between 7 and 90 (got %d)", days)
	}

	// Generate forecast
	forecast, err := engine.GenerateForecast(projectID, model, days)
	if err != nil {
		return fmt.Errorf("failed to generate forecast: %w", err)
	}

	// JSON output
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(forecast)
	}

	// Human-readable output
	scope := "all projects (global)"
	if projectID != "" {
		scope = fmt.Sprintf("project %s", projectID)
	}

	fmt.Printf("📈 %s\n", makeHeader(fmt.Sprintf("Cost Forecast for %s", scope)))
	fmt.Println()

	fmt.Printf("Model:              %s\n", forecast.Model)
	fmt.Printf("Generated:          %s\n", forecast.GeneratedAt.Format(time.RFC3339))
	fmt.Printf("Historical Data:    %d days\n", forecast.HistoricalDays)
	fmt.Printf("Forecast Period:    %d days\n", forecast.ForecastDays)
	fmt.Printf("Base Cost:          $%.2f\n", forecast.BaseCost)
	fmt.Printf("Base Date:          %s\n", forecast.BaseDate.Format("2006-01-02"))
	fmt.Println()

	fmt.Printf("📊 %s\n", makeHeader("Cost Predictions"))
	if forecast.Predicted7Days > 0 {
		fmt.Printf("   7 days:          $%.2f\n", forecast.Predicted7Days)
	}
	if forecast.Predicted14Days > 0 {
		fmt.Printf("   14 days:         $%.2f\n", forecast.Predicted14Days)
	}
	if forecast.Predicted30Days > 0 {
		fmt.Printf("   30 days:         $%.2f\n", forecast.Predicted30Days)
	}
	if forecast.Predicted60Days > 0 {
		fmt.Printf("   60 days:         $%.2f\n", forecast.Predicted60Days)
	}
	if forecast.Predicted90Days > 0 {
		fmt.Printf("   90 days:         $%.2f\n", forecast.Predicted90Days)
	}
	fmt.Println()

	// Confidence intervals
	if forecast.Confidence30Days != nil {
		fmt.Printf("🎯 %s\n", makeHeader("30-Day Confidence Interval (95%)"))
		fmt.Printf("   Prediction:      $%.2f\n", forecast.Confidence30Days.Prediction)
		fmt.Printf("   Lower Bound:     $%.2f\n", forecast.Confidence30Days.LowerBound)
		fmt.Printf("   Upper Bound:     $%.2f\n", forecast.Confidence30Days.UpperBound)
		fmt.Println()
	}

	// Model accuracy
	fmt.Printf("✅ %s\n", makeHeader("Model Accuracy"))
	fmt.Printf("   R² Score:        %.3f (%.1f%%)\n", forecast.R2Score, forecast.R2Score*100)
	fmt.Printf("   MAE:             $%.2f\n", forecast.MeanAbsoluteError)
	fmt.Printf("   RMSE:            $%.2f\n", forecast.RootMeanSquaredError)
	fmt.Printf("   Accuracy:        %.1f%%\n", forecast.ModelAccuracy*100)
	fmt.Println()

	return nil
}

// runBurnrate analyzes burn rates with trend detection
func runBurnrate(ctx context.Context, region, projectID string, days int, jsonOutput bool) error {
	// Load cost manager
	costMgr, err := loadCostManagerWithRegion(ctx, region)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	// Create forecast engine
	engine := cost.NewForecastEngine(costMgr.GetReporter())

	// Validate days
	if days < 7 || days > 365 {
		return fmt.Errorf("days must be between 7 and 365 (got %d)", days)
	}

	// Analyze burn rate
	analysis, err := engine.AnalyzeBurnRate(projectID, days)
	if err != nil {
		return fmt.Errorf("failed to analyze burn rate: %w", err)
	}

	// JSON output
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(analysis)
	}

	// Human-readable output
	scope := "all projects (global)"
	if projectID != "" {
		scope = fmt.Sprintf("project %s", projectID)
	}

	fmt.Printf("🔥 %s\n", makeHeader(fmt.Sprintf("Burn Rate Analysis for %s", scope)))
	fmt.Println()

	fmt.Printf("📊 %s\n", makeHeader("Current Burn Rates"))
	fmt.Printf("   Daily:           $%.2f/day\n", analysis.CurrentDailyRate)
	if analysis.CurrentWeeklyRate > 0 {
		fmt.Printf("   Weekly:          $%.2f/week\n", analysis.CurrentWeeklyRate)
	}
	if analysis.CurrentMonthlyRate > 0 {
		fmt.Printf("   Monthly:         $%.2f/month\n", analysis.CurrentMonthlyRate)
	}
	fmt.Println()

	fmt.Printf("📈 %s\n", makeHeader("Historical Statistics"))
	fmt.Printf("   Average:         $%.2f/day\n", analysis.AverageDailyRate)
	fmt.Printf("   Min:             $%.2f/day\n", analysis.MinDailyRate)
	fmt.Printf("   Max:             $%.2f/day\n", analysis.MaxDailyRate)
	fmt.Printf("   Std Dev:         $%.2f\n", analysis.StdDevDailyRate)
	fmt.Printf("   Volatility:      %.1f%%\n", analysis.Volatility*100)
	fmt.Println()

	fmt.Printf("📉 %s\n", makeHeader("Trend Analysis"))
	var trendIcon string
	switch analysis.TrendDirection {
	case "increasing":
		trendIcon = "📈"
	case "decreasing":
		trendIcon = "📉"
	default:
		trendIcon = "➡️"
	}
	fmt.Printf("   Direction:       %s %s\n", trendIcon, analysis.TrendDirection)
	fmt.Printf("   Strength:        %.1f%%\n", analysis.TrendStrength*100)
	fmt.Printf("   Acceleration:    $%.2f/day²\n", analysis.AccelerationRate)
	fmt.Println()

	fmt.Printf("🔮 %s\n", makeHeader("Predicted Burn Rates"))
	fmt.Printf("   30 days:         $%.2f/day\n", analysis.PredictedDailyRate30Days)
	fmt.Printf("   60 days:         $%.2f/day\n", analysis.PredictedDailyRate60Days)
	fmt.Printf("   90 days:         $%.2f/day\n", analysis.PredictedDailyRate90Days)
	fmt.Println()

	// Confidence intervals
	if ci95, ok := analysis.ConfidenceIntervals[95]; ok {
		fmt.Printf("🎯 %s\n", makeHeader("Confidence Interval (95%)"))
		fmt.Printf("   Prediction:      $%.2f/day\n", ci95.Prediction)
		fmt.Printf("   Lower Bound:     $%.2f/day\n", ci95.LowerBound)
		fmt.Printf("   Upper Bound:     $%.2f/day\n", ci95.UpperBound)
		fmt.Println()
	}

	return nil
}

// runExhaustion predicts when budget will be exhausted
func runExhaustion(ctx context.Context, region, projectID string, budget, spent float64, jsonOutput bool) error {
	// Load cost manager
	costMgr, err := loadCostManagerWithRegion(ctx, region)
	if err != nil {
		return fmt.Errorf("failed to load cost manager: %w", err)
	}

	// Create forecast engine
	engine := cost.NewForecastEngine(costMgr.GetReporter())

	// If spent not provided, calculate from cost records
	if spent == 0 {
		if projectID != "" {
			spent = costMgr.GetReporter().GetProjectCosts(projectID)
		} else {
			// Get global costs (current month as default)
			spent = costMgr.GetReporter().GetCurrentMonthCosts()
		}
	}

	// Validate budget
	if budget <= 0 {
		return fmt.Errorf("budget must be > 0 (got %.2f)", budget)
	}

	// Predict exhaustion
	forecast, err := engine.PredictBudgetExhaustion(projectID, budget, spent)
	if err != nil {
		return fmt.Errorf("failed to predict budget exhaustion: %w", err)
	}

	// JSON output
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(forecast)
	}

	// Human-readable output
	scope := "all projects (global)"
	if projectID != "" {
		scope = fmt.Sprintf("project %s", projectID)
	}

	fmt.Printf("⚠️  %s\n", makeHeader(fmt.Sprintf("Budget Exhaustion Prediction for %s", scope)))
	fmt.Println()

	fmt.Printf("💰 %s\n", makeHeader("Budget Status"))
	fmt.Printf("   Total Budget:    $%.2f\n", budget)
	fmt.Printf("   Current Spend:   $%.2f\n", spent)
	fmt.Printf("   Remaining:       $%.2f\n", budget-spent)
	fmt.Printf("   Usage:           %.1f%%\n", (spent/budget)*100)
	fmt.Println()

	// Exhaustion prediction
	fmt.Printf("📅 %s\n", makeHeader("Exhaustion Forecast"))
	if forecast.BudgetExhaustionDate != nil {
		if forecast.DaysUntilExhaustion == 0 {
			fmt.Printf("   Status:          ⚠️  BUDGET EXHAUSTED\n")
			fmt.Printf("   Exhausted On:    %s (today)\n", forecast.BudgetExhaustionDate.Format("2006-01-02"))
			fmt.Printf("   Probability:     %.0f%%\n", forecast.ExhaustionProbability*100)
		} else if forecast.DaysUntilExhaustion > 0 {
			fmt.Printf("   Exhaustion Date: %s\n", forecast.BudgetExhaustionDate.Format("2006-01-02"))
			fmt.Printf("   Days Until:      %d days\n", forecast.DaysUntilExhaustion)
			fmt.Printf("   Probability:     %.1f%%\n", forecast.ExhaustionProbability*100)

			// Warning levels
			if forecast.DaysUntilExhaustion <= 7 {
				fmt.Printf("   Warning:         🔴 CRITICAL - Budget exhausts within a week\n")
			} else if forecast.DaysUntilExhaustion <= 14 {
				fmt.Printf("   Warning:         🟡 HIGH - Budget exhausts within 2 weeks\n")
			} else if forecast.DaysUntilExhaustion <= 30 {
				fmt.Printf("   Warning:         🟠 MEDIUM - Budget exhausts within a month\n")
			} else {
				fmt.Printf("   Warning:         🟢 LOW - Budget exhausts in %d days\n", forecast.DaysUntilExhaustion)
			}
		}
	} else {
		fmt.Printf("   Status:          ✅ Budget will NOT exhaust within 90 days\n")
		fmt.Printf("   Probability:     %.1f%%\n", forecast.ExhaustionProbability*100)
	}
	fmt.Println()

	// Forecast details
	fmt.Printf("📈 %s\n", makeHeader("Cost Forecast"))
	fmt.Printf("   Model:           %s\n", forecast.Model)
	fmt.Printf("   Base Cost:       $%.2f\n", forecast.BaseCost)
	if forecast.Predicted30Days > 0 {
		fmt.Printf("   30-day Forecast: $%.2f\n", forecast.Predicted30Days)
	}
	if forecast.Predicted90Days > 0 {
		fmt.Printf("   90-day Forecast: $%.2f\n", forecast.Predicted90Days)
	}
	fmt.Println()

	return nil
}

// loadCostManagerWithRegion loads cost manager with specific region
func loadCostManagerWithRegion(ctx context.Context, region string) (*cost.Manager, error) {
	// Load AWS config with region
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Load CargoShip config
	cargoConfig := cargoconfig.DefaultAWSConfig()

	// Create cost manager
	costMgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cost manager: %w", err)
	}

	return costMgr, nil
}

// runBenchmarkCompare calculates cost comparison for benchmark scenarios (Issue #165)
func runBenchmarkCompare(
	ctx context.Context,
	region, tool string,
	sizeGB float64,
	fileCount int64,
	compressionRatio, dedupRatio float64,
	storageClass cargoconfig.StorageClass,
	showChart bool,
) error {
	// Load AWS config
	awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, region)
	if err != nil {
		return err
	}

	// Create pricing manager
	pricingMgr, err := cost.NewPricingManager(&cargoConfig.CostControl.Pricing, awsCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create pricing manager: %w", err)
	}

	// Create benchmark calculator
	benchmarkCalc := cost.NewBenchmarkCostCalculator(pricingMgr, region)

	var result interface{}
	var cargoshipCost *cost.BenchmarkCostComparison
	var competitorCost *cost.BenchmarkCostComparison

	if tool == "" || tool == "cargoship" {
		// Calculate CargoShip cost with optimizations
		cargoshipCost, err = benchmarkCalc.CalculateCargoShipCost(
			ctx,
			"benchmark",
			sizeGB,
			fileCount,
			compressionRatio,
			dedupRatio,
			storageClass,
		)
		if err != nil {
			return fmt.Errorf("failed to calculate CargoShip cost: %w", err)
		}

		if tool == "cargoship" {
			result = cargoshipCost
		} else {
			// Calculate competitor cost for comparison
			competitorCost, err = benchmarkCalc.CalculateCompetitorCost(
				ctx,
				"benchmark",
				"competitor",
				sizeGB,
				fileCount,
			)
			if err != nil {
				return fmt.Errorf("failed to calculate competitor cost: %w", err)
			}

			// Compare costs
			comparison := benchmarkCalc.CompareCosts(cargoshipCost, competitorCost)
			result = comparison
		}
	} else {
		// Calculate competitor cost only
		competitorCost, err = benchmarkCalc.CalculateCompetitorCost(
			ctx,
			"benchmark",
			tool,
			sizeGB,
			fileCount,
		)
		if err != nil {
			return fmt.Errorf("failed to calculate cost: %w", err)
		}
		result = competitorCost
	}

	// Display ASCII charts if requested
	if showChart {
		fmt.Println()
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println("                         COST BENCHMARK COMPARISON")
		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()

		// Show comparison table if we have both costs
		if cargoshipCost != nil && competitorCost != nil {
			fmt.Print(cost.ComparisonTable(cargoshipCost, competitorCost))
			fmt.Println()
			fmt.Print(cost.CostComparisonChart(cargoshipCost.AnnualTCO, competitorCost.AnnualTCO, "CargoShip", "Competitor"))
			fmt.Println()
		}

		// Show CargoShip advantages if available
		if cargoshipCost != nil && cargoshipCost.CargoShipAdvantages != nil {
			fmt.Print(cost.SavingsBreakdownChart(cargoshipCost.CargoShipAdvantages))
			fmt.Println()
		}

		// Show monthly vs annual projection
		if cargoshipCost != nil {
			fmt.Print(cost.MonthlyVsAnnualChart(
				cargoshipCost.MonthlyRunningCost,
				cargoshipCost.TotalUploadCost,
				cargoshipCost.AnnualTCO,
			))
		} else if competitorCost != nil {
			fmt.Print(cost.MonthlyVsAnnualChart(
				competitorCost.MonthlyRunningCost,
				competitorCost.TotalUploadCost,
				competitorCost.AnnualTCO,
			))
		}

		fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Println()

		return nil
	}

	// Output as JSON (default behavior for script integration)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// runCostSummary implements `cargoship cost summary` (Issue #186).
func runCostSummary(ctx context.Context, region, dvcStage, gitCommit string, jsonOutput bool) error {
	awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, region)
	if err != nil {
		return err
	}

	costMgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create cost manager: %w", err)
	}

	if dvcStage != "" {
		summary, err := costMgr.QueryCostsByDVCStage(dvcStage)
		if err != nil {
			return err
		}

		if jsonOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(summary)
		}

		fmt.Printf("DVC Stage: %s\n", summary.DVCStage)
		fmt.Printf("  Total cost:    $%.4f %s\n", summary.TotalCost, summary.Currency)
		fmt.Printf("  Total size:    %.3f GB\n", summary.TotalSizeGB)
		fmt.Printf("  Records:       %d\n", summary.RecordCount)
		fmt.Printf("  First run:     %s\n", summary.FirstRun.Format(time.RFC3339))
		fmt.Printf("  Last run:      %s\n", summary.LastRun.Format(time.RFC3339))
		if len(summary.ByCommit) > 0 {
			fmt.Println("  By commit:")
			for commit, c := range summary.ByCommit {
				fmt.Printf("    %s  $%.4f\n", commit, c)
			}
		}
		return nil
	}

	// gitCommit path
	records, err := costMgr.QueryCostsByGitCommit(gitCommit)
	if err != nil {
		return err
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(records)
	}

	totalCost := 0.0
	for _, r := range records {
		totalCost += r.Cost
	}
	fmt.Printf("Git commit: %s\n", gitCommit)
	fmt.Printf("  Records:    %d\n", len(records))
	fmt.Printf("  Total cost: $%.4f\n", totalCost)
	fmt.Println("  Records:")
	for _, r := range records {
		dvcInfo := ""
		if r.DVCStage != "" {
			dvcInfo = fmt.Sprintf(" [stage: %s]", r.DVCStage)
		}
		fmt.Printf("    %s  %s  $%.4f%s\n",
			r.Timestamp.Format("2006-01-02T15:04:05Z"),
			r.FileName,
			r.Cost,
			dvcInfo,
		)
	}
	return nil
}

// runComplianceReport implements `cargoship cost report --format compliance` (Issue #187).
func runComplianceReport(ctx context.Context, region, budgetID, grantNumber, agency, outputFile string, jsonOutput bool) error {
	if budgetID == "" {
		return fmt.Errorf("--budget is required for compliance reports")
	}
	if grantNumber == "" {
		return fmt.Errorf("--grant is required for compliance reports")
	}

	awsCfg, cargoConfig, err := loadAWSConfigForCost(ctx, region)
	if err != nil {
		return err
	}

	costMgr, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, slog.Default())
	if err != nil {
		return fmt.Errorf("failed to create cost manager: %w", err)
	}

	var report *cost.ComplianceReport
	switch strings.ToUpper(agency) {
	case "NIH":
		report, err = costMgr.GenerateNIHComplianceReport(budgetID, grantNumber)
	default: // NSF
		report, err = costMgr.GenerateNSFComplianceReport(budgetID, grantNumber)
	}
	if err != nil {
		return err
	}

	var output string
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if outputFile != "" {
			f, ferr := os.Create(outputFile)
			if ferr != nil {
				return fmt.Errorf("failed to create output file: %w", ferr)
			}
			defer func() { _ = f.Close() }()
			enc = json.NewEncoder(f)
			enc.SetIndent("", "  ")
		}
		return enc.Encode(report)
	}

	output = cost.FormatComplianceReportText(report)
	if outputFile != "" {
		if werr := os.WriteFile(outputFile, []byte(output), 0o644); werr != nil {
			return fmt.Errorf("failed to write output file: %w", werr)
		}
		fmt.Fprintf(os.Stderr, "Compliance report written to %s\n", outputFile)
		return nil
	}
	fmt.Print(output)
	return nil
}
