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
		region       string
		jsonOutput   bool

		// Estimate flags
		size         string
		storageClass string
		operation    string

		// Report flags
		period       string
		outputFile   string
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
	budgetCmd := &cobra.Command{
		Use:   "budget",
		Short: "Show budget status and spending",
		Long: `Display budget status, spending, and alerts.

Shows:
- Maximum budget configured
- Current spending
- Remaining budget
- Budget usage percentage
- Alert status

Examples:
  # Show budget status
  cargoship cost budget

  # Budget status as JSON
  cargoship cost budget --json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBudget(cmd.Context(), region, jsonOutput)
		},
	}

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
		Long: `Generate detailed cost report for a time period.

Shows:
- Total costs and savings
- Cost breakdown by service and region
- Cost trends and projections
- Cost optimization recommendations

Examples:
  # Monthly report
  cargoship cost report --period month

  # Weekly report
  cargoship cost report --period week

  # Export report to file
  cargoship cost report --period month --output report.json
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReport(cmd.Context(), region, period, outputFile, jsonOutput)
		},
	}
	reportCmd.Flags().StringVar(&period, "period", "month", "Report period (today, week, month, last_month)")
	reportCmd.Flags().StringVar(&outputFile, "output", "", "Output file path (default: stdout)")

	// Add subcommands
	cmd.AddCommand(estimateCmd, uploadCmd, budgetCmd, pricingCmd, reportCmd)

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

func runBudget(ctx context.Context, region string, jsonOutput bool) error {
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

	// Get budget status
	status := costMgr.GetBudgetStatus()

	// Display results
	if jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(status)
	}

	// Human-readable output with Issue #147 Phase 1 support for custom budget periods
	fmt.Printf("📊 %s\n", makeHeader("Budget Status"))

	maxBudget := status["max_budget"].(float64)
	currentSpend := status["current_spend"].(float64)
	remaining := status["budget_remaining"].(float64)
	usedPercent := status["budget_used"].(float64) * 100
	alertThreshold := status["alert_threshold"].(float64) * 100

	// Check if this is a new budget period (has period_type field) or legacy monthly budget
	periodType, hasPeriodType := status["period_type"]
	if hasPeriodType && periodType != nil {
		// New budget period system
		fmt.Printf("   Budget Period:    %s\n", status["period_type"])
		if periodStart, ok := status["period_start"].(string); ok {
			fmt.Printf("   Period Start:     %s\n", periodStart)
		}
		if periodEnd, ok := status["period_end"].(string); ok {
			fmt.Printf("   Period End:       %s\n", periodEnd)
		}
		if grantName, ok := status["grant_name"].(string); ok && grantName != "" {
			fmt.Printf("   Grant:            %s\n", grantName)
		}
		fmt.Printf("   Max Budget:       $%.2f\n", maxBudget)
		fmt.Printf("   Current Spend:    $%.2f\n", currentSpend)
		fmt.Printf("   Remaining:        $%.2f\n", remaining)
		fmt.Printf("   Usage:            %.1f%%\n", usedPercent)
		fmt.Printf("   Alert Threshold:  %.1f%%\n", alertThreshold)

		// Visual progress bar
		barWidth := 40
		filledWidth := int(usedPercent / 100.0 * float64(barWidth))
		if filledWidth > barWidth {
			filledWidth = barWidth
		}
		bar := strings.Repeat("█", filledWidth) + strings.Repeat("░", barWidth-filledWidth)
		fmt.Printf("   Progress:         [%s] %.1f%%\n", bar, usedPercent)
		fmt.Println()

		// Budget period projection
		if daysElapsed, ok := status["days_elapsed"].(int); ok {
			if daysRemaining, ok := status["days_remaining"].(int); ok {
				if totalDays, ok := status["total_days"].(int); ok {
					if burnRate, ok := status["daily_burn_rate"].(float64); ok {
						if projectedSpend, ok := status["projected_eop_spend"].(float64); ok {
							fmt.Printf("📈 %s\n", makeHeader("Budget Period Projection"))
							fmt.Printf("   Days Elapsed:       %d of %d\n", daysElapsed, totalDays)
							fmt.Printf("   Days Remaining:     %d\n", daysRemaining)
							fmt.Printf("   Daily Burn Rate:    $%.2f/day\n", burnRate)
							if targetRate, ok := status["target_daily_rate"].(float64); ok {
								fmt.Printf("   Target Daily Rate:  $%.2f/day\n", targetRate)
							}
							fmt.Printf("   Projected EOP:      $%.2f\n", projectedSpend)

							if willExceed, ok := status["will_exceed_budget"].(bool); ok && willExceed {
								if overage, ok := status["projected_overage"].(float64); ok {
									overagePercent, _ := status["projected_overage_percent"].(float64)
									fmt.Printf("   Projected Overage:  $%.2f (%.1f%% over budget)\n", overage, overagePercent)
								}
							} else {
								if savings, ok := status["projected_savings"].(float64); ok {
									savingsPercent, _ := status["projected_savings_percent"].(float64)
									fmt.Printf("   Projected Savings:  $%.2f (%.1f%% under budget)\n", savings, savingsPercent)
								}
							}
							fmt.Println()
						}
					}
				}
			}
		}
	} else {
		// Legacy monthly budget system
		fmt.Printf("   Max Budget:       $%.2f/month\n", maxBudget)
		fmt.Printf("   Current Spend:    $%.2f\n", currentSpend)
		fmt.Printf("   Remaining:        $%.2f\n", remaining)
		fmt.Printf("   Usage:            %.1f%%\n", usedPercent)
		fmt.Printf("   Alert Threshold:  %.1f%%\n", alertThreshold)

		// Visual progress bar
		barWidth := 40
		filledWidth := int(usedPercent / 100.0 * float64(barWidth))
		if filledWidth > barWidth {
			filledWidth = barWidth
		}
		bar := strings.Repeat("█", filledWidth) + strings.Repeat("░", barWidth-filledWidth)
		fmt.Printf("   Progress:         [%s] %.1f%%\n", bar, usedPercent)
		fmt.Println()

		// Monthly projection for legacy system
		now := time.Now()
		dayOfMonth := now.Day()
		daysInMonth := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, now.Location()).Day()
		daysRemaining := daysInMonth - dayOfMonth

		if dayOfMonth > 0 {
			dailyBurnRate := currentSpend / float64(dayOfMonth)
			projectedEOM := currentSpend + (dailyBurnRate * float64(daysRemaining))

			fmt.Printf("📈 %s\n", makeHeader("Monthly Projection"))
			fmt.Printf("   Day of Month:       %d of %d\n", dayOfMonth, daysInMonth)
			fmt.Printf("   Daily Burn Rate:    $%.2f/day\n", dailyBurnRate)
			fmt.Printf("   Projected EOM:      $%.2f\n", projectedEOM)

			if projectedEOM > maxBudget {
				overage := projectedEOM - maxBudget
				fmt.Printf("   Projected Overage:  $%.2f (%.1f%% over budget)\n", overage, (overage/maxBudget)*100)
			} else {
				underBudget := maxBudget - projectedEOM
				fmt.Printf("   Under Budget:       $%.2f (%.1f%% savings)\n", underBudget, (underBudget/maxBudget)*100)
			}
			fmt.Println()
		}
	}

	// Status indicator with enhanced recommendations
	if status["over_budget"].(bool) {
		fmt.Printf("🚨 %s\n", makeHeader("STATUS: OVER BUDGET"))
		fmt.Printf("   ⚠️  You have exceeded your monthly budget limit.\n")
		fmt.Println()
		fmt.Printf("💡 Recommendations:\n")
		fmt.Printf("   • Review recent uploads and identify cost drivers\n")
		fmt.Printf("   • Consider switching to INTELLIGENT_TIERING storage class\n")
		fmt.Printf("   • Enable lifecycle policies to transition old data to cheaper tiers\n")
		fmt.Printf("   • Increase monthly budget if current spending is justified\n")
		fmt.Printf("   • Set up cost alerts to catch overspending earlier\n")
	} else if status["alert_triggered"].(bool) {
		fmt.Printf("⚠️  %s\n", makeHeader("STATUS: ALERT THRESHOLD EXCEEDED"))
		fmt.Printf("   You are approaching your budget limit (%.1f%% used).\n", usedPercent)
		fmt.Println()
		fmt.Printf("💡 Recommendations:\n")
		fmt.Printf("   • Monitor daily spending for remainder of month\n")
		fmt.Printf("   • Review planned uploads and defer non-critical transfers\n")
		fmt.Printf("   • Check for any unexpected cost spikes\n")
		fmt.Printf("   • Consider storage class optimizations\n")
	} else {
		fmt.Printf("✅ %s\n", makeHeader("STATUS: WITHIN BUDGET"))
		fmt.Printf("   Budget usage is healthy (%.1f%% used).\n", usedPercent)
		fmt.Println()
		fmt.Printf("💡 Cost Optimization Tips:\n")
		fmt.Printf("   • Continue monitoring spending throughout the month\n")
		fmt.Printf("   • Review compression ratios to maximize storage savings\n")
		fmt.Printf("   • Consider INTELLIGENT_TIERING for rarely accessed data\n")
	}
	fmt.Println()

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
			"upload_id":           m.UploadID,
			"region":              m.Region,
			"created_at":          m.CreatedAt,
			"duration_days":       durationDays,
			"uncompressed_size_gb": uncompressedSizeGB,
			"compressed_size_gb":   compressedSizeGB,
			"compression_ratio":    m.CompressionRatio,
			"monthly_cost":         monthlyCost,
			"total_spent":          totalSpent,
			"uncompressed_cost":    uncompressedCost,
			"savings_per_month":    savingsPerMonth,
			"savings_percent":      savingsPercent,
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
