// Package main provides a CLI tool for CargoShip cost management
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/scttfrdmn/cargoship/pkg/aws/cost"
	cargoconfig "github.com/scttfrdmn/cargoship/pkg/aws/config"
	"gopkg.in/yaml.v3"
)

var (
	configFile    = flag.String("config", "", "Path to CargoShip configuration file")
	command       = flag.String("command", "", "Command to execute: estimate, report, budget, pricing, validate")
	operation     = flag.String("operation", "upload", "Operation type for estimate (upload, download)")
	size          = flag.String("size", "", "Size for estimate (e.g., 1GB, 500MB, 2TB)")
	storageClass  = flag.String("storage-class", "STANDARD", "Storage class (STANDARD, STANDARD_IA, GLACIER, etc.)")
	region        = flag.String("region", "us-east-1", "AWS region")
	period        = flag.String("period", "month", "Report period (today, week, month, last_month)")
	format        = flag.String("format", "json", "Output format (json, csv, table)")
	output        = flag.String("output", "", "Output file path (default: stdout)")
	verbose       = flag.Bool("verbose", false, "Enable verbose logging")
	awsProfile    = flag.String("aws-profile", "", "AWS profile to use")
)

func main() {
	flag.Parse()

	// Setup logging
	logLevel := slog.LevelInfo
	if *verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))

	if *command == "" {
		fmt.Fprintf(os.Stderr, "Error: command is required\n")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.Background()

	// Load configuration
	var cargoConfig *cargoconfig.AWSConfig
	if *configFile != "" {
		cfg, err := loadConfig(*configFile)
		if err != nil {
			logger.Error("Failed to load configuration", "error", err)
			os.Exit(1)
		}
		cargoConfig = cfg
	} else {
		cargoConfig = cargoconfig.DefaultAWSConfig()
	}

	// Override with command line flags
	if *awsProfile != "" {
		cargoConfig.Profile = *awsProfile
	}
	if *region != "" {
		cargoConfig.Region = *region
	}

	// Load AWS configuration
	awsCfg, err := cargoconfig.LoadAWSConfig(ctx, cargoConfig.Profile, cargoConfig.Region)
	if err != nil {
		logger.Error("Failed to load AWS configuration", "error", err)
		os.Exit(1)
	}

	// Create cost manager
	costManager, err := cost.NewManager(&cargoConfig.CostControl, awsCfg, logger)
	if err != nil {
		logger.Error("Failed to create cost manager", "error", err)
		os.Exit(1)
	}

	// Execute command
	switch *command {
	case "estimate":
		err = handleEstimate(ctx, costManager, logger)
	case "report":
		err = handleReport(ctx, costManager, logger)
	case "budget":
		err = handleBudget(costManager, logger)
	case "pricing":
		err = handlePricing(ctx, costManager, logger)
	case "validate":
		err = handleValidate(costManager, logger)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown command: %s\n", *command)
		flag.Usage()
		os.Exit(1)
	}

	if err != nil {
		logger.Error("Command failed", "command", *command, "error", err)
		os.Exit(1)
	}
}

func handleEstimate(ctx context.Context, costManager *cost.Manager, logger *slog.Logger) error {
	if *size == "" {
		return fmt.Errorf("size is required for estimate command")
	}

	// Parse size
	sizeBytes, err := parseSize(*size)
	if err != nil {
		return fmt.Errorf("invalid size format: %w", err)
	}
	sizeGB := float64(sizeBytes) / (1024 * 1024 * 1024)

	// Parse storage class
	storageClassEnum, err := parseStorageClass(*storageClass)
	if err != nil {
		return fmt.Errorf("invalid storage class: %w", err)
	}

	// Get cost estimate
	estimate, err := costManager.EstimateOperationCost(ctx, *operation, sizeGB, storageClassEnum, *region)
	if err != nil {
		return fmt.Errorf("failed to estimate cost: %w", err)
	}

	// Format output
	switch *format {
	case "json":
		return outputJSON(estimate)
	case "table":
		return outputEstimateTable(estimate, sizeGB, storageClassEnum)
	default:
		return fmt.Errorf("unsupported format for estimate: %s", *format)
	}
}

func handleReport(ctx context.Context, costManager *cost.Manager, logger *slog.Logger) error {
	// Generate report
	summary, err := costManager.GenerateCostReport(ctx, *period)
	if err != nil {
		return fmt.Errorf("failed to generate cost report: %w", err)
	}

	// Output or export report
	if *output != "" {
		return costManager.ExportCostReport(ctx, summary, *format, *output)
	}

	// Format output to stdout
	switch *format {
	case "json":
		return outputJSON(summary)
	case "table":
		return outputReportTable(summary)
	default:
		return fmt.Errorf("unsupported format for report: %s", *format)
	}
}

func handleBudget(costManager *cost.Manager, logger *slog.Logger) error {
	status := costManager.GetBudgetStatus()

	switch *format {
	case "json":
		return outputJSON(status)
	case "table":
		return outputBudgetTable(status)
	default:
		return fmt.Errorf("unsupported format for budget: %s", *format)
	}
}

func handlePricing(ctx context.Context, costManager *cost.Manager, logger *slog.Logger) error {
	pricing, err := costManager.GetCurrentPricing(ctx, *region)
	if err != nil {
		return fmt.Errorf("failed to get current pricing: %w", err)
	}

	switch *format {
	case "json":
		return outputJSON(pricing)
	case "table":
		return outputPricingTable(pricing)
	default:
		return fmt.Errorf("unsupported format for pricing: %s", *format)
	}
}

func handleValidate(costManager *cost.Manager, logger *slog.Logger) error {
	err := costManager.ValidateConfig()
	if err != nil {
		fmt.Printf("Configuration validation failed: %v\n", err)
		return err
	}

	fmt.Println("Configuration validation passed ✓")
	return nil
}

// Helper functions

func loadConfig(filename string) (*cargoconfig.AWSConfig, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config cargoconfig.AWSConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
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

func outputJSON(v interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(v)
}

func outputEstimateTable(estimate *cost.CostEstimate, sizeGB float64, storageClass cargoconfig.StorageClass) error {
	fmt.Printf("Cost Estimate\n")
	fmt.Printf("=============\n")
	fmt.Printf("Size:            %.2f GB\n", sizeGB)
	fmt.Printf("Storage Class:   %s\n", storageClass)
	fmt.Printf("Currency:        %s\n", estimate.Currency)
	fmt.Printf("\n")
	fmt.Printf("Cost Breakdown:\n")
	fmt.Printf("  Storage Cost:      $%.4f\n", estimate.StorageCost)
	fmt.Printf("  Request Cost:      $%.4f\n", estimate.RequestCost)
	fmt.Printf("  Data Transfer:     $%.4f\n", estimate.DataTransferCost)
	fmt.Printf("  ---------------\n")
	fmt.Printf("  Total Cost:        $%.4f\n", estimate.TotalCost)
	fmt.Printf("\n")
	fmt.Printf("Discounts Applied:\n")
	fmt.Printf("  Original Cost:     $%.4f\n", estimate.Discounts.OriginalCost)
	fmt.Printf("  Total Discount:    $%.4f\n", estimate.Discounts.TotalDiscount)
	fmt.Printf("  Final Cost:        $%.4f\n", estimate.Discounts.DiscountedCost)
	return nil
}

func outputReportTable(summary *cost.CostSummary) error {
	fmt.Printf("Cost Report - %s\n", summary.Period)
	fmt.Printf("==================\n")
	fmt.Printf("Total Cost:      $%.2f %s\n", summary.TotalCost, summary.Currency)
	fmt.Printf("Total Savings:   $%.2f %s\n", summary.TotalSavings, summary.Currency)
	fmt.Printf("\n")
	
	fmt.Printf("By Service:\n")
	for service, cost := range summary.ByService {
		fmt.Printf("  %-15s $%.2f\n", service, cost)
	}
	fmt.Printf("\n")
	
	fmt.Printf("By Region:\n")
	for region, cost := range summary.ByRegion {
		fmt.Printf("  %-15s $%.2f\n", region, cost)
	}
	fmt.Printf("\n")
	
	fmt.Printf("Trends:\n")
	fmt.Printf("  Daily Average:     $%.2f\n", summary.Trends.DailyAverage)
	fmt.Printf("  Weekly Average:    $%.2f\n", summary.Trends.WeeklyAverage)
	fmt.Printf("  Monthly Projection: $%.2f\n", summary.Trends.MonthlyProjection)
	fmt.Printf("  Cost per GB:       $%.4f\n", summary.Trends.CostPerGB)
	
	if len(summary.Recommendations) > 0 {
		fmt.Printf("\nRecommendations:\n")
		for _, rec := range summary.Recommendations {
			fmt.Printf("  [%s] %s\n", strings.ToUpper(rec.Priority), rec.Description)
			fmt.Printf("      Potential Saving: $%.2f\n", rec.PotentialSaving)
		}
	}
	
	return nil
}

func outputBudgetTable(status map[string]interface{}) error {
	fmt.Printf("Budget Status\n")
	fmt.Printf("=============\n")
	fmt.Printf("Max Budget:      $%.2f\n", status["max_budget"])
	fmt.Printf("Current Spend:   $%.2f\n", status["current_spend"])
	fmt.Printf("Remaining:       $%.2f\n", status["budget_remaining"])
	fmt.Printf("Usage:           %.1f%%\n", status["budget_used"].(float64)*100)
	fmt.Printf("Alert Threshold: %.1f%%\n", status["alert_threshold"].(float64)*100)
	
	if status["over_budget"].(bool) {
		fmt.Printf("\n⚠️  OVER BUDGET!\n")
	} else if status["alert_triggered"].(bool) {
		fmt.Printf("\n⚠️  Budget alert threshold exceeded!\n")
	} else {
		fmt.Printf("\n✅ Within budget\n")
	}
	
	return nil
}

func outputPricingTable(pricing map[string]interface{}) error {
	fmt.Printf("Current AWS Pricing - %s\n", pricing["region"])
	fmt.Printf("========================\n")
	fmt.Printf("Currency: %s\n", pricing["currency"])
	fmt.Printf("Source: %s\n", pricing["source"])
	fmt.Printf("Last Updated: %s\n", pricing["last_updated"])
	fmt.Printf("\n")
	
	if storage, ok := pricing["storage_per_gb_month"].(map[string]float64); ok {
		fmt.Printf("Storage Pricing (per GB per month):\n")
		for class, price := range storage {
			fmt.Printf("  %-20s $%.6f\n", class, price)
		}
		fmt.Printf("\n")
	}
	
	if requests, ok := pricing["requests"].(map[string]float64); ok {
		fmt.Printf("Request Pricing:\n")
		for reqType, price := range requests {
			fmt.Printf("  %-20s $%.6f\n", reqType, price)
		}
	}
	
	return nil
}