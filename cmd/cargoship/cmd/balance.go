package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/spf13/cobra"

	"github.com/scttfrdmn/cargoship/pkg/manifest"
	"github.com/scttfrdmn/cargoship/pkg/pipeline"
)

var (
	balanceFormat    string
	balanceThreshold float64
	balanceProfile   string
	balanceRegion    string
	balanceExecute   bool
	balanceDryRun    bool
)

// NewBalanceCmd creates the balance command for analyzing shard balance
func NewBalanceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "balance <s3://bucket/prefix/uploads/upload-id>",
		Short: "Analyze shard balance for an uploaded dataset",
		Long: `Analyze and rebalance shard distribution for a CargoShip upload.

This command downloads the manifest for an upload and analyzes the distribution
of files across shards. It identifies imbalanced shards and can automatically
rebalance them by redistributing files.

An upload is considered imbalanced if the largest shard is more than 2x the average
shard size (configurable with --threshold).

Examples:
  # Analyze shard balance only (read-only)
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123

  # Show what would be rebalanced without executing
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123 --dry-run

  # Execute rebalancing (redistributes files across shards)
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123 --execute

  # Check with custom threshold (3x instead of 2x)
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123 --threshold 3.0

  # Output as JSON
  cargoship balance s3://my-bucket/data/uploads/20240101-abc123 --format json`,
		Args: cobra.ExactArgs(1),
		RunE: runBalance,
	}

	cmd.Flags().StringVarP(&balanceFormat, "format", "f", "table", "Output format (table, json)")
	cmd.Flags().Float64Var(&balanceThreshold, "threshold", 2.0, "Imbalance threshold (max/avg ratio)")
	cmd.Flags().StringVar(&balanceProfile, "profile", "", "AWS profile to use")
	cmd.Flags().StringVar(&balanceRegion, "region", "", "AWS region")
	cmd.Flags().BoolVar(&balanceExecute, "execute", false, "Execute rebalancing (modifies data)")
	cmd.Flags().BoolVar(&balanceDryRun, "dry-run", false, "Show rebalancing plan without executing")

	// Mark execute and dry-run as mutually exclusive
	cmd.MarkFlagsMutuallyExclusive("execute", "dry-run")

	return cmd
}

func runBalance(cmd *cobra.Command, args []string) error {
	s3URL := args[0]

	// Parse S3 URL to extract bucket, prefix, and upload ID
	// Expected format: s3://bucket/prefix/uploads/upload-id
	bucket, fullPrefix, err := parseBalanceS3URL(s3URL)
	if err != nil {
		return fmt.Errorf("invalid S3 URL: %w", err)
	}

	// Extract upload ID and prefix from full path
	// Format: prefix/uploads/upload-id
	uploadID, prefix, err := extractUploadID(fullPrefix)
	if err != nil {
		return fmt.Errorf("invalid upload path: %w (expected format: prefix/uploads/upload-id)", err)
	}

	ctx := context.Background()

	// Load AWS configuration
	awsCfg, err := loadAWSConfigForBalance(ctx, balanceProfile, balanceRegion)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client
	s3Client := s3.NewFromConfig(awsCfg)

	// Download manifest
	fmt.Printf("📥 Downloading manifest from s3://%s/%s/uploads/%s/manifest.json.gz\n", bucket, prefix, uploadID)

	m, err := manifest.DownloadFromS3(ctx, s3Client, bucket, prefix, uploadID)
	if err != nil {
		return fmt.Errorf("failed to download manifest: %w", err)
	}

	fmt.Printf("✓ Manifest loaded: %d files across %d shards\n\n", m.TotalFiles, m.ShardCount)

	// If execute or dry-run, perform rebalancing
	if balanceExecute || balanceDryRun {
		return runRebalancing(ctx, m, s3Client, bucket, prefix, uploadID, balanceDryRun)
	}

	// Otherwise, just analyze balance
	balance, err := pipeline.AnalyzeShardBalance(m, balanceThreshold)
	if err != nil {
		return fmt.Errorf("failed to analyze balance: %w", err)
	}

	// Output results
	switch balanceFormat {
	case "json":
		return outputBalanceJSON(balance)
	case "table":
		pipeline.PrintShardBalance(balance)
		return nil
	default:
		return fmt.Errorf("unsupported format: %s", balanceFormat)
	}
}

// runRebalancing performs the rebalancing operation
func runRebalancing(ctx context.Context, m *manifest.Manifest, s3Client *s3.Client, bucket, prefix, uploadID string, dryRun bool) error {
	// Configure rebalancing
	config := &pipeline.RebalanceConfig{
		ImbalanceThreshold: balanceThreshold,
		MinShardSize:       100 * 1024 * 1024, // 100MB
		DryRun:             dryRun,
		S3Client:           s3Client,
		Bucket:             bucket,
		Prefix:             prefix,
		UploadID:           uploadID,
		CompressionType:    m.CompressionType,
		StorageClass:       inferStorageClass(m),
	}

	// Perform rebalancing
	result, err := pipeline.RebalanceShards(ctx, m, config)
	if err != nil {
		return fmt.Errorf("rebalancing failed: %w", err)
	}

	// Display results
	fmt.Printf("\n📊 Initial Balance:\n")
	pipeline.PrintShardBalance(&result.InitialBalance)

	if dryRun {
		fmt.Printf("\n🔍 Dry Run Results:\n")
		fmt.Printf("════════════════════════════════════════\n\n")
		if result.Plan != nil {
			fmt.Printf("Files to move:     %d\n", len(result.Plan.Moves))
			fmt.Printf("Total bytes:       %.2f GB\n", float64(result.Plan.TotalBytes)/(1024*1024*1024))
			fmt.Printf("Chunks affected:   %d\n", result.Plan.ChunksAffected)
			fmt.Printf("Shards affected:   %d\n", len(result.ShardsAffected))
			fmt.Printf("\n💡 %s\n", result.Recommendation)
		} else {
			fmt.Printf("💡 %s\n", result.Recommendation)
		}
		return nil
	}

	// Execution mode
	if !result.Success {
		return fmt.Errorf("rebalancing execution failed: %v", result.Error)
	}

	fmt.Printf("\n✅ Final Balance:\n")
	pipeline.PrintShardBalance(&result.FinalBalance)

	fmt.Printf("\n📝 Summary:\n")
	fmt.Printf("════════════════════════════════════════\n")
	fmt.Printf("Files moved:       %d\n", result.FilesReassigned)
	fmt.Printf("Shards affected:   %d\n", len(result.ShardsAffected))
	fmt.Printf("Imbalance ratio:   %.2fx → %.2fx\n",
		result.InitialBalance.ImbalanceRatio,
		result.FinalBalance.ImbalanceRatio)
	fmt.Printf("\n💡 %s\n\n", result.Recommendation)

	// Upload updated manifest
	fmt.Printf("📤 Uploading updated manifest...\n")
	if err := m.UploadToS3(ctx, s3Client, true); err != nil {
		return fmt.Errorf("failed to upload manifest: %w", err)
	}

	fmt.Printf("✅ Rebalancing complete!\n")
	return nil
}

// inferStorageClass infers storage class from manifest metadata
func inferStorageClass(m *manifest.Manifest) types.StorageClass {
	// Check if any chunks specify a storage class
	// For now, default to STANDARD
	// TODO: Could be enhanced to detect from existing chunks
	return types.StorageClassStandard
}

// parseBalanceS3URL parses an S3 URL into bucket and prefix
func parseBalanceS3URL(s3URL string) (bucket, prefix string, err error) {
	if len(s3URL) < 5 || s3URL[:5] != "s3://" {
		return "", "", fmt.Errorf("URL must start with s3://")
	}

	path := s3URL[5:]
	slashIdx := -1
	for i, c := range path {
		if c == '/' {
			slashIdx = i
			break
		}
	}

	if slashIdx == -1 {
		return path, "", nil
	}

	bucket = path[:slashIdx]
	prefix = path[slashIdx+1:]

	// Remove trailing slash
	if len(prefix) > 0 && prefix[len(prefix)-1] == '/' {
		prefix = prefix[:len(prefix)-1]
	}

	return bucket, prefix, nil
}

// extractUploadID extracts upload ID and base prefix from a full path
// Input:  "data/uploads/20240101-abc123"
// Output: "20240101-abc123", "data"
func extractUploadID(fullPath string) (uploadID, prefix string, err error) {
	// Find "/uploads/" in the path
	uploadsIdx := -1
	for i := 0; i < len(fullPath)-8; i++ {
		if fullPath[i:i+9] == "/uploads/" {
			uploadsIdx = i
			break
		}
	}

	if uploadsIdx == -1 {
		return "", "", fmt.Errorf("path must contain /uploads/")
	}

	prefix = fullPath[:uploadsIdx]
	uploadID = fullPath[uploadsIdx+9:] // Skip "/uploads/"

	if uploadID == "" {
		return "", "", fmt.Errorf("upload ID cannot be empty")
	}

	return uploadID, prefix, nil
}

// loadAWSConfigForBalance loads AWS configuration for balance command
func loadAWSConfigForBalance(ctx context.Context, profile, region string) (aws.Config, error) {
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

// outputBalanceJSON outputs balance analysis in JSON format
func outputBalanceJSON(balance *pipeline.ShardBalance) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(balance)
}
