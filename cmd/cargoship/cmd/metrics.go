package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/aws/metrics"
)

var (
	metricsNamespace string
	metricsRegion    string
	metricsTest      bool
)

// NewMetricsCmd creates the metrics command for CloudWatch integration testing
func NewMetricsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "metrics",
		Short: "Test CloudWatch metrics integration",
		Long: `Test CloudWatch metrics integration for CargoShip observability.

This command allows you to test the CloudWatch metrics publishing functionality
to ensure proper integration with AWS monitoring services.

Examples:
  # Test metrics publishing
  cargoship metrics --test
  
  # Test with custom namespace and region
  cargoship metrics --test --namespace "CargoShip/Prod" --region us-east-1`,
		RunE: runMetrics,
	}

	cmd.Flags().StringVar(&metricsNamespace, "namespace", "CargoShip/Test", "CloudWatch namespace for metrics")
	cmd.Flags().StringVar(&metricsRegion, "region", "us-west-2", "AWS region for CloudWatch")
	cmd.Flags().BoolVar(&metricsTest, "test", false, "Send test metrics to CloudWatch")

	return cmd
}

func runMetrics(cmd *cobra.Command, args []string) error {
	if !metricsTest {
		return fmt.Errorf("use --test flag to send test metrics to CloudWatch")
	}

	fmt.Printf("🔍 Testing CloudWatch metrics integration...\n")
	fmt.Printf("   Namespace: %s\n", metricsNamespace)
	fmt.Printf("   Region: %s\n", metricsRegion)

	// Load AWS config
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(metricsRegion))
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create CloudWatch client
	cloudwatchClient := cloudwatch.NewFromConfig(cfg)

	// Create metrics publisher
	metricsConfig := metrics.MetricConfig{
		Namespace:     metricsNamespace,
		Region:        metricsRegion,
		BatchSize:     5, // Small batch for testing
		FlushInterval: 10 * time.Second,
		Enabled:       true,
	}

	publisher := metrics.NewCloudWatchPublisher(cloudwatchClient, metricsConfig)

	// Test upload metrics
	fmt.Printf("\n📊 Publishing upload metrics...\n")
	uploadMetrics := &metrics.UploadMetrics{
		Duration:        45 * time.Second,
		ThroughputMBps:  15.5,
		TotalBytes:      500 * 1024 * 1024, // 500MB
		ChunkCount:      25,
		Concurrency:     8,
		ErrorCount:      0,
		Success:         true,
		StorageClass:    "INTELLIGENT_TIERING",
		ContentType:     "application/octet-stream",
		CompressionType: "zstd",
	}

	if err := publisher.PublishUploadMetrics(ctx, uploadMetrics); err != nil {
		return fmt.Errorf("failed to publish upload metrics: %w", err)
	}

	// Test cost metrics
	fmt.Printf("💰 Publishing cost metrics...\n")
	costMetrics := &metrics.CostMetrics{
		EstimatedMonthlyCost:    2.30,
		EstimatedAnnualCost:     27.60,
		ActualMonthlyCost:       0.10,
		DataSizeGB:              100.0,
		PotentialSavingsPercent: 95.7,
		StorageClass:            "DEEP_ARCHIVE",
		OptimizationType:        "archive_optimization",
	}

	if err := publisher.PublishCostMetrics(ctx, costMetrics); err != nil {
		return fmt.Errorf("failed to publish cost metrics: %w", err)
	}

	// Test network metrics
	fmt.Printf("🌐 Publishing network metrics...\n")
	networkMetrics := &metrics.NetworkMetrics{
		BandwidthMBps:      25.0,
		LatencyMs:          50.0,
		PacketLossPercent:  0.1,
		OptimalChunkSizeMB: 24,
		OptimalConcurrency: 8,
		NetworkCondition:   "excellent",
	}

	if err := publisher.PublishNetworkMetrics(ctx, networkMetrics); err != nil {
		return fmt.Errorf("failed to publish network metrics: %w", err)
	}

	// Test operational metrics
	fmt.Printf("⚙️ Publishing operational metrics...\n")
	operationalMetrics := &metrics.OperationalMetrics{
		ActiveUploads:    3,
		QueuedUploads:    7,
		CompletedUploads: 45,
		FailedUploads:    2,
		MemoryUsageMB:    256.5,
		CPUUsagePercent:  25.3,
	}

	if err := publisher.PublishOperationalMetrics(ctx, operationalMetrics); err != nil {
		return fmt.Errorf("failed to publish operational metrics: %w", err)
	}

	// Test lifecycle metrics
	fmt.Printf("🔄 Publishing lifecycle metrics...\n")
	lifecycleMetrics := &metrics.LifecycleMetrics{
		ActivePolicies:          1,
		EstimatedSavingsPercent: 95.7,
		ObjectsTransitioned:     1250,
		PolicyTemplate:          "archive-optimization",
		BucketName:              "cargoship-production",
	}

	if err := publisher.PublishLifecycleMetrics(ctx, lifecycleMetrics); err != nil {
		return fmt.Errorf("failed to publish lifecycle metrics: %w", err)
	}

	// Flush any remaining metrics
	fmt.Printf("🚀 Flushing metrics to CloudWatch...\n")
	if err := publisher.Flush(ctx); err != nil {
		return fmt.Errorf("failed to flush metrics: %w", err)
	}

	fmt.Printf("\n✅ All test metrics published successfully!\n")
	fmt.Printf("\n🔍 Check CloudWatch console:\n")
	fmt.Printf("   https://console.aws.amazon.com/cloudwatch/home?region=%s#metricsV2:graph=~();search=%s\n",
		metricsRegion, metricsNamespace)

	fmt.Printf("\n📈 Key metrics published:\n")
	fmt.Printf("   • Upload performance (duration, throughput, errors)\n")
	fmt.Printf("   • Cost optimization (savings, storage classes)\n")
	fmt.Printf("   • Network conditions (bandwidth, latency, optimization)\n")
	fmt.Printf("   • Operational status (uploads, memory, CPU)\n")
	fmt.Printf("   • Lifecycle policies (savings, transitions)\n")

	return nil
}

func init() {
	// This command will be added to root in root.go
}
